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

	"github.com/optivem/gh-optivem/internal/log"
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
		return nil, err
	}

	creds := base64.StdEncoding.EncodeToString([]byte(s.Token + ":"))
	req.Header.Set("Authorization", "Basic "+creds)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var result map[string]interface{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result); err != nil {
			log.Warnf("SonarCloud: failed to parse response JSON: %v", err)
		}
	}
	if result == nil {
		result = make(map[string]interface{})
	}

	if resp.StatusCode >= 400 {
		result["error"] = true
		result["status"] = float64(resp.StatusCode)
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
	return strings.Contains(strings.ToLower(msg), "already exist")
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
	} else {
		log.OKf("SonarCloud org: %s", s.Org)
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
		log.OKf("SonarCloud project: %s", key)
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
		log.OKf("Deleted SonarCloud project: %s", key)
	}
}
