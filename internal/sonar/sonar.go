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
func (c *Client) do(method, endpoint string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("sonar: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sonar: %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sonar: %s %s: HTTP %d: %s",
			method, endpoint, resp.StatusCode, strings.TrimSpace(string(respBody)))
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
