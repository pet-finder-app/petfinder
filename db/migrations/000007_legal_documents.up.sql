CREATE TABLE legal_documents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),kind text NOT NULL CHECK(kind IN ('terms','privacy')),version text NOT NULL,
 title text NOT NULL,content text NOT NULL CHECK(length(btrim(content))>0),reviewed_by text NOT NULL,
 placeholder boolean NOT NULL DEFAULT false,effective_at timestamptz NOT NULL,published_at timestamptz NOT NULL DEFAULT now(),UNIQUE(kind,version)
);
INSERT INTO legal_documents(kind,version,title,content,reviewed_by,placeholder,effective_at) VALUES
 ('terms','0.1-placeholder','Terms of use','Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.','User-requested placeholder',true,now()),
 ('privacy','0.1-placeholder','Privacy policy','Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.','User-requested placeholder',true,now());
CREATE TABLE legal_acceptances(user_id uuid NOT NULL REFERENCES users(id),document_id uuid NOT NULL REFERENCES legal_documents(id),accepted_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,document_id));
