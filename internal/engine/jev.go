package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// JEVConfig 独立配置判断服务，不复用搜索或综合模型的凭据。
type JEVConfig struct {
	APIURL      string `json:"apiURL"`
	APIKey      string `json:"apiKey"`
	Model       string `json:"model"`
	TimeoutSec  int    `json:"timeoutSec"`
	AnswerCheck bool   `json:"answerCheck,omitempty"`
}

// NormalizeJEVConfig 补全默认值；未配置 Key 时仍允许使用固定研究流程。
func NormalizeJEVConfig(cfg JEVConfig) (JEVConfig, error) {
	cfg.APIURL = strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.APIURL == "" {
		cfg.APIURL = "https://api.typesafe.ai/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "jev-latest"
	}
	if cfg.TimeoutSec == 0 {
		cfg.TimeoutSec = 15
	}
	if cfg.TimeoutSec < 1 || cfg.TimeoutSec > 300 {
		return JEVConfig{}, fmt.Errorf("jev timeoutSec must be between 1 and 300")
	}
	u, err := url.Parse(cfg.APIURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return JEVConfig{}, fmt.Errorf("jev apiURL must be an HTTP(S) URL without credentials, query or fragment")
	}
	return cfg, nil
}

// JEVQuestion 描述一个由本地程序定义的概率判断。
type JEVQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// JEVJudgment 只包含请求过的问题；未返回用量时保留 nil。
type JEVJudgment struct {
	Scores       map[string]float64
	InputTokens  *int
	OutputTokens *int
}

// JEVClient 无会话状态，可由并发研究请求共用。
type JEVClient struct {
	HTTPClient *http.Client
	config     JEVConfig
	configErr  error
}

func NewJEVClient(cfg JEVConfig) *JEVClient {
	normalized, err := NormalizeJEVConfig(cfg)
	return &JEVClient{
		config:    normalized,
		configErr: err,
		HTTPClient: &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

// Configured 只报告配置是否完整，不执行网络探测。
func (c *JEVClient) Configured() bool {
	return c != nil && c.configErr == nil && c.config.APIKey != ""
}

// Evaluate 的重试和等待共享单次判断预算，并服从调用方更短的截止时间。
func (c *JEVClient) Evaluate(ctx context.Context, state any, questions map[string]JEVQuestion) (JEVJudgment, error) {
	if c == nil {
		return JEVJudgment{}, fmt.Errorf("jev client is not configured")
	}
	if c.configErr != nil {
		return JEVJudgment{}, c.configErr
	}
	if c.config.APIKey == "" {
		return JEVJudgment{}, fmt.Errorf("jev apiKey is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.config.TimeoutSec)*time.Second)
	defer cancel()
	body, err := json.Marshal(struct {
		Model     string                 `json:"model"`
		State     any                    `json:"state"`
		Questions map[string]JEVQuestion `json:"questions"`
	}{c.config.Model, state, questions})
	if err != nil {
		return JEVJudgment{}, fmt.Errorf("jev request encoding failed")
	}
	endpoint := c.config.APIURL
	if !strings.HasSuffix(endpoint, "/systemone") {
		endpoint += "/systemone"
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return JEVJudgment{}, fmt.Errorf("jev judgment: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return JEVJudgment{}, fmt.Errorf("jev request creation failed")
		}
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return JEVJudgment{}, fmt.Errorf("jev judgment: %w", ctx.Err())
			}
			// 网络错误可能带有 URL；不把凭据或服务返回正文写入日志。
			return JEVJudgment{}, fmt.Errorf("jev request failed")
		}
		const maxResponseBytes = 1 << 20
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		resp.Body.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return JEVJudgment{}, fmt.Errorf("jev judgment: %w", ctx.Err())
			}
			return JEVJudgment{}, fmt.Errorf("jev response read failed")
		}
		if (resp.StatusCode == 429 || resp.StatusCode == 529) && attempt < 2 {
			timer := time.NewTimer(time.Duration(1<<attempt) * 500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return JEVJudgment{}, fmt.Errorf("jev judgment: %w", ctx.Err())
			case <-timer.C:
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return JEVJudgment{}, fmt.Errorf("jev API HTTP %d", resp.StatusCode)
		}
		if len(data) > maxResponseBytes {
			return JEVJudgment{}, fmt.Errorf("jev response exceeds size limit")
		}
		return parseJEVJudgment(data, questions)
	}
	return JEVJudgment{}, fmt.Errorf("jev retries exhausted")
}

func parseJEVJudgment(data []byte, questions map[string]JEVQuestion) (JEVJudgment, error) {
	var response struct {
		Answers map[string]json.RawMessage `json:"answers"`
		Usage   json.RawMessage            `json:"usage"`
	}
	if err := json.Unmarshal(data, &response); err != nil || response.Answers == nil {
		return JEVJudgment{}, fmt.Errorf("jev returned invalid answers")
	}
	judgment := JEVJudgment{Scores: make(map[string]float64, len(questions))}
	for id := range questions {
		var answer struct {
			Type string   `json:"type"`
			Noul *float64 `json:"noul"`
		}
		if err := json.Unmarshal(response.Answers[id], &answer); err != nil || answer.Type != "noul" || answer.Noul == nil || math.IsNaN(*answer.Noul) || math.IsInf(*answer.Noul, 0) || *answer.Noul < 0 || *answer.Noul > 1 {
			return JEVJudgment{}, fmt.Errorf("jev returned an invalid probability")
		}
		judgment.Scores[id] = *answer.Noul
	}
	var usage map[string]json.RawMessage
	if json.Unmarshal(response.Usage, &usage) == nil {
		for key, dst := range map[string]**int{"input_tokens": &judgment.InputTokens, "output_tokens": &judgment.OutputTokens} {
			var count *int
			if json.Unmarshal(usage[key], &count) == nil && count != nil && *count >= 0 {
				*dst = count
			}
		}
	}
	return judgment, nil
}
