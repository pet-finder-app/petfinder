package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type paymentDetails struct {
	Method        string  `json:"method" enum:"pix,bank_transfer"`
	Instructions  string  `json:"instructions" minLength:"1" maxLength:"3000"`
	PixKey        *string `json:"pix_key,omitempty" maxLength:"200"`
	BankName      *string `json:"bank_name,omitempty" maxLength:"120"`
	BankBranch    *string `json:"bank_branch,omitempty" maxLength:"30"`
	BankAccount   *string `json:"bank_account,omitempty" maxLength:"50"`
	AccountHolder string  `json:"account_holder" minLength:"2" maxLength:"200"`
	Published     bool    `json:"published"`
}
type donationRequest struct {
	AmountCents    int64     `json:"amount_cents" minimum:"1" maximum:"1000000000"`
	TransferredAt  time.Time `json:"transferred_at"`
	ReceiptMediaID *string   `json:"receipt_media_id,omitempty" format:"uuid"`
	Anonymous      bool      `json:"anonymous"`
	Note           string    `json:"note,omitempty" maxLength:"2000"`
}
type donationResponse struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	DonorID        *string   `json:"donor_id"`
	DonorName      *string   `json:"donor_name"`
	AmountCents    int64     `json:"amount_cents"`
	Currency       string    `json:"currency"`
	TransferredAt  time.Time `json:"transferred_at"`
	ReceiptMediaID *string   `json:"receipt_media_id"`
	Anonymous      bool      `json:"anonymous"`
	Note           string    `json:"note"`
	Status         string    `json:"status"`
	DecisionReason *string   `json:"decision_reason"`
	CreatedAt      time.Time `json:"created_at"`
}
type donationDecision struct {
	Status string `json:"status" enum:"confirmed,not_found"`
	Reason string `json:"reason" minLength:"5" maxLength:"2000"`
}
type donationTotals struct {
	ReportedCents  int64  `json:"reported_cents"`
	ConfirmedCents int64  `json:"confirmed_cents"`
	NotFoundCents  int64  `json:"not_found_cents"`
	Currency       string `json:"currency"`
}
type donationHistoryRecord struct {
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
type donationDetail struct {
	Donation donationResponse        `json:"donation"`
	History  []donationHistoryRecord `json:"history"`
}

const donationProjection = `jsonb_build_object('id',d.id,'organization_id',d.organization_id,'donor_id',CASE WHEN NOT d.anonymous OR d.donor_id=$1 THEN d.donor_id END,'donor_name',CASE WHEN NOT d.anonymous OR d.donor_id=$1 THEN u.display_name END,'amount_cents',d.amount_cents,'currency',d.currency,'transferred_at',d.transferred_at,'receipt_media_id',d.receipt_media_id,'anonymous',d.anonymous,'note',d.note,'status',d.status,'decision_reason',d.decision_reason,'created_at',d.created_at)`

func registerDonationRoutes(s *Server, reg *routeRegistry) {
	registerRoute[paymentDetails, paymentDetails](reg, "PUT", "/v1/organizations/{organizationId}/payment-details", "updatePaymentDetails", "Set direct-transfer payment instructions", "Donations", 200, s.protected(s.updatePaymentDetails), bearerAuth(false))
	registerRoute[noBody, paymentDetails](reg, "GET", "/v1/organizations/{organizationId}/payment-details", "getPaymentDetails", "Read published direct-transfer instructions", "Donations", 200, s.optionalAuth(s.getPaymentDetails), bearerAuth(true))
	registerRoute[donationRequest, donationResponse](reg, "POST", "/v1/organizations/{organizationId}/donations", "reportDonation", "Report a direct transfer for organization confirmation", "Donations", 201, s.protected(s.reportDonation), bearerAuth(false))
	registerRoute[noBody, collectionResponse[donationResponse]](reg, "GET", "/v1/donations", "listDonations", "List your reported transfers or authorized organization donations", "Donations", 200, s.protected(s.listDonations), bearerAuth(false), queryParameters[paginationQuery], queryParameters[organizationIDQuery])
	registerRoute[noBody, donationTotals](reg, "GET", "/v1/organizations/{organizationId}/donation-totals", "donationTotals", "Read authorized donation totals", "Donations", 200, s.protected(s.donationTotals), bearerAuth(false))
	registerRoute[noBody, donationDetail](reg, "GET", "/v1/donations/{donationId}", "getDonation", "Read a transfer report and decision history", "Donations", 200, s.protected(s.getDonation), bearerAuth(false))
	registerRoute[donationDecision, donationDetail](reg, "POST", "/v1/donations/{donationId}/decision", "decideDonation", "Confirm or mark a reported transfer not found", "Donations", 200, s.protected(s.decideDonation), bearerAuth(false))
}
func (s *Server) updatePaymentDetails(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	if !s.financeAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var req paymentDetails
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Method == "pix" && (req.PixKey == nil || strings.TrimSpace(*req.PixKey) == "") {
		writeServiceError(w, ErrInvalid)
		return
	}
	if req.Method == "bank_transfer" && (req.BankName == nil || req.BankAccount == nil || req.BankBranch == nil) {
		writeServiceError(w, ErrInvalid)
		return
	}
	actor, _ := subjectUUID(r)
	_, err := s.pool.Exec(r.Context(), `INSERT INTO organization_payment_details(organization_id,method,instructions,pix_key,bank_name,bank_branch,bank_account,account_holder,published,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(organization_id) DO UPDATE SET method=EXCLUDED.method,instructions=EXCLUDED.instructions,pix_key=EXCLUDED.pix_key,bank_name=EXCLUDED.bank_name,bank_branch=EXCLUDED.bank_branch,bank_account=EXCLUDED.bank_account,account_holder=EXCLUDED.account_holder,published=EXCLUDED.published,updated_by=EXCLUDED.updated_by,updated_at=now()`, org, req.Method, req.Instructions, req.PixKey, req.BankName, req.BankBranch, req.BankAccount, req.AccountHolder, req.Published, actor)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, req)
}
func (s *Server) getPaymentDetails(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	manager := s.financeAllowed(r, org)
	if !manager && !s.publicOrganization(r, org) {
		writeServiceError(w, ErrNotFound)
		return
	}
	var result paymentDetails
	err := s.pool.QueryRow(r.Context(), `SELECT method,instructions,pix_key,bank_name,bank_branch,bank_account,account_holder,published FROM organization_payment_details WHERE organization_id=$1 AND (published OR $2)`, org, manager).Scan(&result.Method, &result.Instructions, &result.PixKey, &result.BankName, &result.BankBranch, &result.BankAccount, &result.AccountHolder, &result.Published)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) reportDonation(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	if !s.publicOrganization(r, org) {
		writeServiceError(w, ErrNotFound)
		return
	}
	var req donationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TransferredAt.After(time.Now()) || req.TransferredAt.IsZero() {
		writeServiceError(w, ErrInvalid)
		return
	}
	actor, _ := subjectUUID(r)
	var media pgtype.UUID
	if req.ReceiptMediaID != nil {
		if !s.checkMedia(w, r, []string{*req.ReceiptMediaID}, "document") {
			return
		}
		media, _ = pathUUIDValue(*req.ReceiptMediaID)
	}
	var id string
	err := s.pool.QueryRow(r.Context(), `INSERT INTO donations(organization_id,donor_id,amount_cents,transferred_at,receipt_media_id,anonymous,note,payment_snapshot) SELECT $1,$2,$3,$4,$5,$6,$7,to_jsonb(p)-'updated_by' FROM organization_payment_details p WHERE p.organization_id=$1 AND p.published RETURNING id::text`, org, actor, req.AmountCents, req.TransferredAt, media, req.Anonymous, req.Note).Scan(&id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	var v donationResponse
	err = s.pool.QueryRow(r.Context(), `SELECT `+donationProjection+` FROM donations d JOIN users u ON u.id=d.donor_id WHERE d.id=$2`, actor, id).Scan(&v)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 201, v)
}
func (s *Server) listDonations(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	var org pgtype.UUID
	if raw := r.URL.Query().Get("organization_id"); raw != "" {
		var ok bool
		org, ok = pathUUIDValue(raw)
		if !ok {
			invalidPath(w)
			return
		}
		if !s.financeAllowed(r, org) {
			writeServiceError(w, ErrForbidden)
			return
		}
	}
	offset, size, page := pagination(r)
	rows, err := s.pool.Query(r.Context(), `SELECT `+donationProjection+` FROM donations d JOIN users u ON u.id=d.donor_id WHERE (CASE WHEN $2::uuid IS NULL THEN d.donor_id=$1 ELSE d.organization_id=$2 END) ORDER BY d.created_at DESC,d.id LIMIT $3 OFFSET $4`, actor, org, size, offset)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []donationResponse{}
	for rows.Next() {
		var v donationResponse
		if err = rows.Scan(&v); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, 200, collectionResponse[donationResponse]{Items: items, Page: page})
}
func (s *Server) donationScope(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, pgtype.UUID, bool) {
	id, ok := pathUUID(r, "donationId")
	if !ok {
		invalidPath(w)
		return id, pgtype.UUID{}, pgtype.UUID{}, false
	}
	var donor, org pgtype.UUID
	if err := s.pool.QueryRow(r.Context(), "SELECT donor_id,organization_id FROM donations WHERE id=$1", id).Scan(&donor, &org); err != nil {
		handleDBError(w, err)
		return id, donor, org, false
	}
	actor, _ := subjectUUID(r)
	if actor != donor && !s.financeAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return id, donor, org, false
	}
	return id, donor, org, true
}
func (s *Server) getDonation(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := s.donationScope(w, r)
	if !ok {
		return
	}
	actor, _ := subjectUUID(r)
	result := donationDetail{History: []donationHistoryRecord{}}
	if err := s.pool.QueryRow(r.Context(), `SELECT `+donationProjection+` FROM donations d JOIN users u ON u.id=d.donor_id WHERE d.id=$2`, actor, id).Scan(&result.Donation); err != nil {
		handleDBError(w, err)
		return
	}
	rows, err := s.pool.Query(r.Context(), "SELECT status,reason,created_at FROM donation_history WHERE donation_id=$1 ORDER BY id", id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var v donationHistoryRecord
		if err = rows.Scan(&v.Status, &v.Reason, &v.CreatedAt); err != nil {
			handleDBError(w, err)
			return
		}
		result.History = append(result.History, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) decideDonation(w http.ResponseWriter, r *http.Request) {
	id, _, org, ok := s.donationScope(w, r)
	if !ok {
		return
	}
	if !s.financeAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var req donationDecision
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := subjectUUID(r)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	tag, err := tx.Exec(r.Context(), "UPDATE donations SET status=$2,decision_reason=$3,decided_by=$4,decided_at=now() WHERE id=$1 AND status='reported'", id, req.Status, req.Reason, actor)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeServiceError(w, ErrConflict)
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO donation_history(donation_id,status,reason,actor_id) VALUES($1,$2,$3,$4)", id, req.Status, req.Reason, actor); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	s.getDonation(w, r)
}
func (s *Server) donationTotals(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	if !s.financeAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	result := donationTotals{Currency: "BRL"}
	err := s.pool.QueryRow(r.Context(), `SELECT COALESCE(sum(amount_cents) FILTER(WHERE status='reported'),0),COALESCE(sum(amount_cents) FILTER(WHERE status='confirmed'),0),COALESCE(sum(amount_cents) FILTER(WHERE status='not_found'),0) FROM donations WHERE organization_id=$1`, org).Scan(&result.ReportedCents, &result.ConfirmedCents, &result.NotFoundCents)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
