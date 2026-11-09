package accounts

import "net/http"

func (a *Auth) userRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/users", a.listUsers)
	m.HandleFunc("PUT /api/v1/users/{id}", a.editUser)
	m.HandleFunc("POST /api/v1/users/{id}/two-factor/reset", a.resetUserTwoFactor)
	m.HandleFunc("POST /api/v1/users/{id}/password", a.setUserPassword)
	m.HandleFunc("GET /api/v1/users/invites", a.listInvites)
	m.HandleFunc("POST /api/v1/users/invites", a.makeInvite)
	m.HandleFunc("DELETE /api/v1/users/invites/{id}", a.deleteInvite)
	m.HandleFunc("GET /api/v1/auth/invites/{token}", a.lookInvite)
	m.HandleFunc("POST /api/v1/auth/invites/{token}", a.acceptInvite)
	m.HandleFunc("POST /api/v1/auth/password", a.changePassword)
}
