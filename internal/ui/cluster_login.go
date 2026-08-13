package ui

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	reOCToken  = regexp.MustCompile(`(?i)--token(?:=|\s+)(?:"([^"]+)"|'([^']+)'|(\S+))`)
	reOCServer = regexp.MustCompile(`(?i)--server(?:=|\s+)(?:"(https?://[^"]+)"|'(https?://[^']+)'|(https?://\S+))`)
	reCurlAuth = regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+([A-Za-z0-9._~\-+/=]+)`)
	reCurlURL  = regexp.MustCompile(`(?i)https?://[^\s"']+:\d{2,5}`)
	reAPIHost  = regexp.MustCompile(`(?i)https?://api\.[^\s"']+`)
)

func firstGroup(m []string) string {
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			return m[i]
		}
	}
	return ""
}

type parsedLogin struct {
	Server string `json:"server"`
	Token  string `json:"-"`      // never JSON-serialize in responses
	Format string `json:"format"` // oc | curl | mixed
	Name   string `json:"name"`
}

// ParseLoginPaste accepts OpenShift console "Copy login command" paste:
// either `oc login --token=… --server=…`, and/or the curl Bearer form,
// including a paste that contains both blocks.
func ParseLoginPaste(raw string) (*parsedLogin, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("paste an oc login or curl login command from the OpenShift console")
	}

	var ocToken, ocServer, curlToken, curlServer string
	if m := reOCToken.FindStringSubmatch(raw); len(m) > 1 {
		ocToken = firstGroup(m)
	}
	if m := reOCServer.FindStringSubmatch(raw); len(m) > 1 {
		ocServer = strings.TrimRight(firstGroup(m), "/")
	}
	if m := reCurlAuth.FindStringSubmatch(raw); len(m) == 2 {
		curlToken = m[1]
	}
	if m := reCurlURL.FindString(raw); m != "" {
		curlServer = baseAPIServer(m)
	} else if m := reAPIHost.FindString(raw); m != "" {
		curlServer = baseAPIServer(m)
	}

	hasOC := ocToken != "" && ocServer != ""
	hasCurl := curlToken != "" && curlServer != ""

	out := &parsedLogin{}
	switch {
	case hasOC && hasCurl:
		out.Format = "mixed"
		out.Token = ocToken
		out.Server = ocServer
		// Prefer oc fields; fall back to curl if oc incomplete (shouldn't happen here).
		if out.Token == "" {
			out.Token = curlToken
		}
		if out.Server == "" {
			out.Server = curlServer
		}
	case hasOC:
		out.Format = "oc"
		out.Token = ocToken
		out.Server = ocServer
	case hasCurl:
		out.Format = "curl"
		out.Token = curlToken
		out.Server = curlServer
	default:
		// Partial paste: combine pieces across formats.
		out.Token = firstNonEmpty(ocToken, curlToken)
		out.Server = firstNonEmpty(ocServer, curlServer)
		if out.Token == "" || out.Server == "" {
			return nil, fmt.Errorf("could not find both token and server — paste the oc login line and/or the curl Bearer line from Display Token")
		}
		out.Format = "mixed"
	}

	out.Server = strings.TrimRight(out.Server, "/")
	if !strings.HasPrefix(out.Server, "https://") && !strings.HasPrefix(out.Server, "http://") {
		return nil, fmt.Errorf("server must be an https URL, got %q", out.Server)
	}
	if len(out.Token) < 16 {
		return nil, fmt.Errorf("token looks too short")
	}
	out.Name = displayNameFromServer(out.Server)
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func baseAPIServer(rawURL string) string {
	rawURL = strings.Trim(rawURL, `"'`)
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// Strip path heuristically.
		if i := strings.Index(rawURL, "/apis/"); i > 0 {
			return strings.TrimRight(rawURL[:i], "/")
		}
		if i := strings.Index(rawURL, "/api/"); i > 0 {
			return strings.TrimRight(rawURL[:i], "/")
		}
		return strings.TrimRight(rawURL, "/")
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
}

func displayNameFromServer(server string) string {
	u, err := url.Parse(server)
	if err != nil {
		return "target-cluster"
	}
	host := u.Hostname()
	// api.2026-prod-1.ocp.dasmlab.org -> 2026-prod-1
	host = strings.TrimPrefix(host, "api.")
	if i := strings.Index(host, ".ocp."); i > 0 {
		return host[:i]
	}
	if i := strings.Index(host, "."); i > 0 {
		return host[:i]
	}
	if host == "" {
		return "target-cluster"
	}
	return host
}

func (s *Server) clustersDir() string {
	dir := filepath.Join(s.RunDir, "clusters")
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

func (s *Server) addClusterLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Paste string `json:"paste"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	parsed, err := ParseLoginPaste(body.Paste)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(body.Name) != "" {
		parsed.Name = sanitizeClusterName(body.Name)
	}

	if err := s.verifyToken(parsed.Server, parsed.Token); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("token check failed against %s: %w", parsed.Server, err))
		return
	}

	path, ctxName, err := s.writeTokenKubeconfig(parsed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	cs := s.clusterState()
	cs.mu.Lock()
	cs.kubeconfig = path
	cs.context = ctxName
	cs.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"added": map[string]any{
			"name":   parsed.Name,
			"server": parsed.Server,
			"format": parsed.Format,
			"source": "login-command",
		},
		"current":  s.currentCluster(),
		"clusters": s.listClusters(),
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func sanitizeClusterName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, name)
	if name == "" {
		return "target-cluster"
	}
	if len(name) > 48 {
		name = name[:48]
	}
	return name
}

func (s *Server) writeTokenKubeconfig(p *parsedLogin) (string, string, error) {
	ctxName := p.Name
	clusterName := p.Name
	userName := p.Name + "-token"
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                p.Server,
		InsecureSkipTLSVerify: true, // lab / self-signed common on OCP edges
	}
	cfg.AuthInfos[userName] = &clientcmdapi.AuthInfo{Token: p.Token}
	cfg.Contexts[ctxName] = &clientcmdapi.Context{
		Cluster:  clusterName,
		AuthInfo: userName,
	}
	cfg.CurrentContext = ctxName

	path := filepath.Join(s.clustersDir(), sanitizeClusterName(p.Name)+".kubeconfig")
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		return "", "", err
	}
	_ = os.Chmod(path, 0o600)
	return path, ctxName, nil
}

func (s *Server) verifyToken(server, token string) error {
	u := strings.TrimRight(server, "/") + "/apis/user.openshift.io/v1/users/~"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // lab clusters
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		// Fallback: core API version probe (vanilla k8s without OpenShift user API).
		return s.verifyTokenK8s(server, token)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return s.verifyTokenK8s(server, token)
	}
	return fmt.Errorf("HTTP %d from users/~", resp.StatusCode)
}

func (s *Server) verifyTokenK8s(server, token string) error {
	u := strings.TrimRight(server, "/") + "/api"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from /api", resp.StatusCode)
	}
	return nil
}
