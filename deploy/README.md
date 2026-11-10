# Deploy para VPS

O workflow executa testes unitários, lint (incluindo a build tag `e2e`), testes e2e HTTP/WebSocket e validação OpenAPI. Somente um `push` para `main`, com todas essas verificações aprovadas, publica a imagem no GHCR e executa o deploy. Pull requests e outras branches executam apenas as verificações.

A imagem contém a API HTTP, o servidor WebSocket, o executável de migrações e os arquivos de schema. PostgreSQL roda como outro container do mesmo Compose, com volume persistente e sem porta publicada no host. A API mantém uma única réplica para o hub de chat em memória. As imagens são publicadas para VPS Linux amd64 e arm64.

## Configuração no GitHub

Crie o environment `production` e configure os secrets e variables abaixo nele (ou no repositório):

| Tipo | Nome | Valor |
|---|---|---|
| Secret | `VPS_HOST` | Hostname ou IPv4 do VPS |
| Secret | `VPS_USER` | Usuário SSH com acesso ao Docker |
| Secret | `VPS_SSH_KEY` | Chave privada SSH sem passphrase, dedicada ao deploy |
| Variable | `VPS_DEPLOY_PATH` | Diretório absoluto, por exemplo `/opt/petfinder`, sem espaços |
| Variable | `VPS_PORT` | Porta SSH, opcional; padrão `22` |

O job de publicação usa `GITHUB_TOKEN` com `packages: write`; não é necessário um PAT para publicar. Caso o pacote GHCR já exista, dê a este repositório acesso de escrita nas configurações do pacote.

O SSH usa `StrictHostKeyChecking=accept-new`: aceita automaticamente a chave do servidor na primeira conexão e rejeita alterações durante o mesmo deploy. O arquivo `known_hosts` é temporário e não persiste entre execuções; portanto, cada deploy confia novamente na primeira conexão, sem verificar previamente a identidade do VPS. Não é necessário configurar `VPS_KNOWN_HOSTS`.

## Preparação do VPS

Instale Docker Engine, o plugin Docker Compose v2 (com suporte a `up --wait`), Bash e tar. O usuário SSH deve conseguir executar `docker` sem sudo e escrever no diretório escolhido. Exemplo para `/opt/petfinder`:

```sh
sudo install -d -o "$USER" -g "$(id -gn)" -m 750 /opt/petfinder
# Copie deploy/.env.example deste repositório para /opt/petfinder/.env.
chmod 600 /opt/petfinder/.env
```

Edite `.env` antes do primeiro deploy: substitua `POSTGRES_PASSWORD` e `JWT_SECRET`, e atualize a senha correspondente em `DATABASE_URL`. A URL aponta para `db:5432`; caracteres especiais da senha precisam de URL encoding nessa URL. O arquivo permanece no VPS e não é enviado pelo workflow. As credenciais PostgreSQL inicializam um volume novo; alterar `.env` depois não altera a senha de um banco já existente.

Para imagens privadas, autentique uma vez no VPS, como o mesmo usuário usado pelo SSH, com um PAT de uma conta que possa ler o pacote e tenha `read:packages` (autorize SSO quando necessário):

```sh
docker login ghcr.io --username SEU_USUARIO_GITHUB
# No prompt Password, informe o PAT. Não o coloque na linha de comando.
```

Configure um proxy HTTPS no host apontando para `127.0.0.1:8080` (ou `API_PORT`), com suporte a WebSocket: encaminhe os cabeçalhos `Upgrade` e `Connection` e permita conexões longas. REST e WebSocket usam a mesma porta. O banco trafega apenas na rede privada do Compose; a configuração de exemplo usa `sslmode=disable` nessa rede local.

## Execução e recuperação

Cada release copia apenas `compose.production.yaml` e `deploy/run.sh` para `<VPS_DEPLOY_PATH>/releases/<commit>`. O script puxa a imagem pelo digest publicado, aguarda o banco ficar saudável, executa as migrações e recria a API, aguardando `/health`. Falhas de migração ou saúde falham o job; a API existente só é substituída após migrações bem-sucedidas. Não há rollback automático: migrações devem manter compatibilidade com a versão anterior durante o deploy, e o restart da API interrompe conexões WebSocket abertas.

O projeto Compose fixo `petfinder-production` preserva o volume `petfinder-production_postgres-data` entre releases. Ele é separado do Compose de desenvolvimento. Para migrar um banco existente, restaure um backup nesse volume antes do primeiro deploy. O workflow nunca executa `down -v` nem remove imagens ou volumes.

`<VPS_DEPLOY_PATH>/current-image` registra o último digest que passou pelo healthcheck. Para inspecionar um deploy, no diretório da release:

```sh
export API_IMAGE=$(cat /opt/petfinder/current-image)
docker compose -p petfinder-production --env-file /opt/petfinder/.env -f compose.production.yaml ps
docker compose -p petfinder-production --env-file /opt/petfinder/.env -f compose.production.yaml logs --tail=100 api
```

O rerun do job `deploy` reaplica a release e as migrações pendentes. Para restaurar uma imagem anterior compatível com o schema atual, execute no diretório da release anterior:

```sh
bash deploy/run.sh /opt/petfinder ghcr.io/OWNER/REPOSITORY@sha256:DIGEST_ANTERIOR
```

Substitua o caminho, nome de imagem e digest pelos valores reais. Faça backups do PostgreSQL antes de migrações destrutivas.
