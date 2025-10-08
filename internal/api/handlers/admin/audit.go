package admin

import (
	"net/http"
	"strconv"
	"time"
)

// GET /admin/audit
func (h *Handler) ListAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	page, size = validatePagination(page, size)

	filter := AuditFilter{
		ActorID:  q.Get("actor_id"),
		TargetID: q.Get("target_id"),
		Action:   q.Get("action"),
		Since:    parseTimeParam(q.Get("since")),
		Until:    parseTimeParam(q.Get("until")),
		Page:     page,
		Size:     size,
	}

	items, total, err := h.Sto.ListAudit(r.Context(), filter)
	if err != nil {
		writeError(w, 500, "audit_list_failed")
		return
	}

	writeJSON(w, 200, map[string]any{
		"data": items, "total": total, "page": page, "size": size,
	})
}

func parseTimeParam(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}
