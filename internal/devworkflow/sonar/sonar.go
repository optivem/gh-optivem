// Package sonar wraps the SonarCloud REST endpoints the cleanup command
// uses (api/projects/search, api/projects/delete). It ports the curl-based
// helpers in github-utils/scripts/delete-sonar-projects.sh.
//
// Auth: every call sends `Authorization: Bearer <token>`. Callers obtain
// the token from $SONAR_TOKEN — same env var the scaffolder reads.
package sonar

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/shell"
)

// DefaultBaseURL is the public SonarCloud API root. Tests and the
// $SONAR_API_URL escape hatch can override via NewClient.
const DefaultBaseURL = "https://sonarcloud.io/api"

// Client talks to SonarCloud. Reuse a single Client per run.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient builds a Client. An empty baseURL resolves to DefaultBaseURL;
// callers can pass $SONAR_API_URL when set. An empty token is rejected by
// every method — fail-fast rather than guessing.
func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Project is the subset of fields the cleanup command consumes from
// projects/search.
type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// SearchPage is the shape of one projects/search response page.
type SearchPage struct {
	Components []Project `json:"components"`
	Paging     struct {
		PageIndex int `json:"pageIndex"`
		PageSize  int `json:"pageSize"`
		Total     int `json:"total"`
	} `json:"paging"`
}

// SearchProjects returns one page of projects for the given organization.
// pageSize is forwarded as `ps`, page as `p` — SonarCloud's own pagination
// parameter names.
func (c *Client) SearchProjects(organization string, page, pageSize int) (*SearchPage, error) {
	if c.Token == "" {
		return nil, errors.New("sonar: SONAR_TOKEN is not set")
	}
	q := url.Values{}
	q.Set("organization", organization)
	q.Set("p", fmt.Sprintf("%d", page))
	q.Set("ps", fmt.Sprintf("%d", pageSize))
	endpoint := c.BaseURL + "/projects/search?" + q.Encode()

	body, err := c.do("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	var p SearchPage
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("sonar: parse projects/search response: %w", err)
	}
	return &p, nil
}

// DeleteProject calls api/projects/delete with `project=<key>` as a
// form-encoded body, matching the bash script's `-d "project=<key>"`.
func (c *Client) DeleteProject(projectKey string) error {
	if c.Token == "" {
		return errors.New("sonar: SONAR_TOKEN is not set")
	}
	endpoint := c.BaseURL + "/projects/delete"
	form := url.Values{}
	form.Set("project", projectKey)
	if _, err := c.do("POST", endpoint, strings.NewReader(form.Encode())); err != nil {
		return err
	}
	return nil
}

// do performs an HTTP call with bearer auth and returns the body bytes
// on a 2xx. Non-2xx responses produce an error whose body is included
// verbatim — that's what callers want to surface to the operator.
//
// Wrapped in shell.RetryWithPolicy with the SonarCloud transient/hard-fail
// regex so 5xx and network errors retry (4 attempts, 5s/15s/45s backoff)
// while 4xx errors propagate immediately. Mirrors the bash sonar-retry.sh
// policy applied to scanner invocations.
func (c *Client) do(method, endpoint string, body io.Reader) ([]byte, error) {
	// Buffer the body once so retries can re-read it. Without this, attempt 2
	// would send an empty body (the strings.Reader from attempt 1 is
	// exhausted). nil body stays nil.
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("sonar: read request body: %w", err)
		}
	}

	var respBody []byte
	var statusCode int

	_, retryErr := shell.RetryWithPolicy(
		shell.RetryTransient(), shell.RetryHardFail(), "sonar-retry",
		func() (string, error) {
			var bodyReader io.Reader
			if bodyBytes != nil {
				bodyReader = strings.NewReader(string(bodyBytes))
			}
			req, err := http.NewRequest(method, endpoint, bodyReader)
			if err != nil {
				return "", fmt.Errorf("sonar: build request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+c.Token)
			if bodyReader != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			resp, err := c.HTTP.Do(req)
			if err != nil {
				wrapped := fmt.Errorf("sonar: %s %s: %w", method, endpoint, err)
				return wrapped.Error(), wrapped
			}
			defer resp.Body.Close()
			respBody, _ = io.ReadAll(resp.Body)
			statusCode = resp.StatusCode
			summary := fmt.Sprintf("HTTP %d\n%s", statusCode, string(respBody))
			// Surface 5xx as an error so the classifier inspects `summary`
			// and retries. 2xx and 4xx fall through to the post-loop check
			// below — 4xx becomes an immediate hard-fail there.
			if statusCode >= 500 {
				return summary, fmt.Errorf("sonar: %s %s: HTTP %d: %s",
					method, endpoint, statusCode, strings.TrimSpace(string(respBody)))
			}
			return summary, nil
		})
	if retryErr != nil && statusCode == 0 {
		// Transport-level failure that never reached a response — return as-is.
		return nil, retryErr
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("sonar: %s %s: HTTP %d: %s",
			method, endpoint, statusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

// MaxPage returns the index of the last page given a total result count and
// page size. Returns 0 when total is 0.
func MaxPage(total, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}
