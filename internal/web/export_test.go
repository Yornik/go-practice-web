package web

// Route is a route of the mux and who may use it, for the gate test.
type Route struct {
	Pattern string
	// Access is "public", "user" (a session, verified or not), "verified"
	// (behind the email gate) or "admin" (behind the gate, admins only).
	Access string
}

// Routes returns every route the server registered, in order.
func (s *Server) Routes() []Route {
	names := map[access]string{public: "public", signedIn: "user", verified: "verified", adminOnly: "admin"}
	out := make([]Route, 0, len(s.routeTable))
	for _, r := range s.routeTable {
		out = append(out, Route{Pattern: r.pattern, Access: names[r.access]})
	}
	return out
}
