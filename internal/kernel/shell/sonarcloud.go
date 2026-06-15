package shell

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/log"
)

// SonarCloud wraps SonarCloud API calls.
type SonarCloud struct {
	Token  string
	Org    string
	client *http.Client
}

func NewSonarCloud(token, org string) *SonarCloud {
	return &SonarCloud{
		Token: token,
		Org:   org,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *SonarCloud) api(ctx context.Context, method, endpoint string, data map[string]string) (map[string]interface{}, error) {
	apiURL := "https://sonarcloud.io/api" + endpoint
	creds := base64.StdEncoding.EncodeToString([]byte(s.Token + ":"))

	var raw []byte
	var statusCode int

	// Wrap Do + ReadAll in RetryWithPolicy so transient 5xx/network failures
	// get the same 4-attempt 5s/15s/45s backoff bash sonar-retry.sh applies
	// to scanner invocations. The fn rebuilds the request body each attempt
	// so retries don't read from an exhausted reader. 4xx is hard-fail —
	// callers handle "already exists" / 404 via the result map below, exactly
	// as before.
	_, retryErr := RetryWithPolicy(retryTransient, retryHardFail, "sonar-retry", func() (string, error) {
		var body io.Reader
		if method == "POST" && data != nil {
			vals := url.Values{}
			for k, v := range data {
				vals.Set(k, v)
			}
			body = strings.NewReader(vals.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, apiURL, body)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Basic "+creds)
		if body != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return err.Error(), err
		}
		defer resp.Body.Close()
		raw, err = io.ReadAll(resp.Body)
		if err != nil {
			return err.Error(), fmt.Errorf("reading response body: %w", err)
		}
		statusCode = resp.StatusCode
		summary := fmt.Sprintf("HTTP %d\n%s", statusCode, string(raw))
		// Synthesize an error on 5xx so the classifier inspects `summary`
		// and retries. 2xx and 4xx return nil here; the result map below
		// surfaces the 4xx to the caller as it always has.
		if statusCode >= 500 {
			return summary, fmt.Errorf("HTTP %d", statusCode)
		}
		return summary, nil
	})
	if retryErr != nil && statusCode == 0 {
		// Transport / body-read error never reached a response. Preserve the
		// pre-retry contract: (nil, err).
		return nil, retryErr
	}
	// Otherwise we have a response (success, 4xx, or exhausted-5xx). Fall
	// through to result-building so callers see the same map shape as today.

	var result map[string]interface{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result); err != nil {
			log.Warnf("SonarCloud: failed to parse response JSON: %v", err)
		}
	}
	if result == nil {
		result = make(map[string]interface{})
	}

	if statusCode >= 400 {
		result["error"] = true
		result["status"] = float64(statusCode)
		if result["message"] == nil {
			result["message"] = string(raw)
		}
	}

	return result, nil
}

func (s *SonarCloud) isAlreadyExists(result map[string]interface{}) bool {
	if err, ok := result["error"]; !ok || err != true {
		return false
	}
	msg, _ := result["message"].(string)
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "already exist") || strings.Contains(lower, "already used")
}

func (s *SonarCloud) CreateOrg() {
	ctx := context.Background()
	result, err := s.api(ctx, "POST", "/organizations/create", map[string]string{
		"key": s.Org, "name": s.Org,
	})
	if err != nil {
		log.Warnf("SonarCloud org creation failed: %v", err)
		return
	}
	if e, ok := result["error"]; ok && e == true && !s.isAlreadyExists(result) {
		log.Warnf("SonarCloud org creation: %v", result["message"])
		return
	}
	if s.isAlreadyExists(result) {
		log.Successf("SonarCloud org (already existed): %s", s.Org)
	} else {
		log.Successf("SonarCloud org (created): %s", s.Org)
	}
}

func (s *SonarCloud) CreateProject(key string) {
	ctx := context.Background()
	result, err := s.api(ctx, "POST", "/projects/create", map[string]string{
		"organization": s.Org, "project": key, "name": key,
	})
	if err != nil {
		log.Warnf("SonarCloud project %s creation failed: %v", key, err)
		return
	}
	if e, ok := result["error"]; ok && e == true && !s.isAlreadyExists(result) {
		log.Warnf("SonarCloud project %s: %v", key, result["message"])
	} else {
		if s.isAlreadyExists(result) {
			log.Successf("SonarCloud project (already existed): %s", key)
		} else {
			log.Successf("SonarCloud project (created): %s", key)
		}
		log.Successf("  https://sonarcloud.io/project/overview?id=%s", key)
	}

	// Rename default branch master -> main
	result, err = s.api(ctx, "POST", "/project_branches/rename", map[string]string{
		"project": key, "name": "main",
	})
	if err != nil {
		log.Warnf("SonarCloud branch rename for %s failed: %v", key, err)
		return
	}
	if e, ok := result["error"]; ok && e == true && !s.isAlreadyExists(result) {
		log.Warnf("SonarCloud branch rename for %s: %v", key, result["message"])
	}
}

// OrgExists reports whether a SonarCloud organization with the given key
// is visible to the authenticated client. Returns (true, nil) when the
// org is found, (false, nil) when the search returns no matches, and
// (false, err) on transport / authentication / unexpected-status failures.
//
// Used by the runtime preflight to surface "the SonarCloud org you
// declared in gh-optivem.yaml doesn't exist" before any agent dispatch
// touches a runner that requires it.
func (s *SonarCloud) OrgExists(ctx context.Context, key string) (bool, error) {
	endpoint := "/organizations/search?organizations=" + url.QueryEscape(key)
	result, err := s.api(ctx, "GET", endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("sonarcloud: organizations/search %s: %w", key, err)
	}
	if e, ok := result["error"]; ok && e == true {
		status, _ := result["status"].(float64)
		msg, _ := result["message"].(string)
		return false, fmt.Errorf("sonarcloud: organizations/search %s: HTTP %d: %s", key, int(status), msg)
	}
	orgs, _ := result["organizations"].([]interface{})
	return len(orgs) > 0, nil
}

// ProjectExists reports whether a SonarCloud project with the given key
// is visible to the authenticated client. Uses /components/show because
// it returns a clean 404 for missing keys (no need to scan a search
// result for the exact match). Returns (true, nil) on 200, (false, nil)
// on 404, (false, err) on every other status or transport failure.
func (s *SonarCloud) ProjectExists(ctx context.Context, key string) (bool, error) {
	endpoint := "/components/show?component=" + url.QueryEscape(key)
	result, err := s.api(ctx, "GET", endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("sonarcloud: components/show %s: %w", key, err)
	}
	if e, ok := result["error"]; ok && e == true {
		status, _ := result["status"].(float64)
		if int(status) == 404 {
			return false, nil
		}
		msg, _ := result["message"].(string)
		return false, fmt.Errorf("sonarcloud: components/show %s: HTTP %d: %s", key, int(status), msg)
	}
	return true, nil
}

func (s *SonarCloud) DeleteProject(key string) {
	ctx := context.Background()
	result, err := s.api(ctx, "POST", "/projects/delete", map[string]string{"project": key})
	if err != nil {
		log.Warnf("SonarCloud project %s deletion failed: %v", key, err)
		return
	}
	if e, ok := result["error"]; ok && e == true {
		log.Warnf("SonarCloud project %s deletion: %v", key, result["message"])
	} else {
		log.Successf("Deleted SonarCloud project: %s", key)
	}
}
