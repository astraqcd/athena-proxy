package daemon

import (
	"encoding/json"
	"mime"
	"net"
	"net/http"
	"os"

	"github.com/astraqcd/athena-proxy/internal/control"
)

func (d *Daemon) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+control.PathStatus, d.handleStatus)
	mux.HandleFunc("GET "+control.PathTargets, d.handleList)
	mux.HandleFunc("POST "+control.PathTargets, d.handleAdd)
	mux.HandleFunc("DELETE "+control.PathTargets+"/{selector}", d.handleRemove)
	return guard(mux)
}

func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, "requests must address the control port as 127.0.0.1 or localhost")
			return
		}
		if r.Method != http.MethodGet && !jsonContentType(r.Header.Get("Content-Type")) {
			writeError(w, http.StatusUnsupportedMediaType, "mutations require Content-Type: application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(host string) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	if name == "localhost" {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

func jsonContentType(header string) bool {
	mediaType, _, err := mime.ParseMediaType(header)
	return err == nil && mediaType == "application/json"
}

func (d *Daemon) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, control.Status{
		Service: control.Service,
		Version: d.opts.Version,
		PID:     os.Getpid(),
	})
}

func (d *Daemon) handleList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, control.ListResponse{Targets: d.List()})
}

func (d *Daemon) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req control.AddRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	resp, err := d.Add(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (d *Daemon) handleRemove(w http.ResponseWriter, r *http.Request) {
	removed, err := d.Remove(r.PathValue("selector"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, control.RemoveResponse{Target: removed})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, control.Error{Message: message})
}
