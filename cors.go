package zmqwebproxy

import "net/http"

func AddCORSHandler(w http.ResponseWriter, r *http.Request) {
	addCORS(w, r)
	w.WriteHeader(http.StatusOK)
}

func CORSMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addCORS(w, r)
		next(w, r)
	}
}

func addCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("origin")
	if len(origin) == 0 {
		host := r.Host
		if len(host) > 0 {
			origin = "https://" + host
		} else {
			origin = "http://localhost"
		}
	}

	w.Header().Set("Access-Control-Allow-Origin", origin) // Replace with your desired origin
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
