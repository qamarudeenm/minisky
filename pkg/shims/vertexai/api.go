package vertexai

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("aiplatform.googleapis.com", func(ctx *registry.Context) http.Handler {
		return &API{
			svcMgr:    ctx.SvcMgr,
			provider: "ollama",
			endpoint: "http://localhost:11434",
			model:    "llama3",
		}
	})
}

type API struct {
	mu        sync.Mutex
	svcMgr    *orchestrator.ServiceManager
	provider  string // ollama, openai
	endpoint  string
	apiKey    string
	model     string
}

type VertexRequest struct {
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

type VertexResponse struct {
	Candidates []struct {
		Content struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle internal config updates from the UI
	if path == "/v1/internal/config" && r.Method == "POST" {
		api.handleConfigUpdate(w, r)
		return
	}

	// publishers/google/models/{model}:generateContent
	if strings.Contains(path, ":generateContent") || strings.Contains(path, ":streamGenerateContent") {
		api.handleGenerateContent(w, r)
		return
	}

	// Handle internal model list discovery
	if path == "/v1/internal/models" && r.Method == "GET" {
		api.handleListModels(w, r)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (api *API) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var cfg struct {
		Provider string `json:"provider"`
		Endpoint string `json:"endpoint"`
		ApiKey   string `json:"apiKey"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	api.mu.Lock()
	api.provider = cfg.Provider
	api.endpoint = cfg.Endpoint
	api.apiKey = cfg.ApiKey
	api.model = cfg.Model
	api.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (api *API) handleGenerateContent(w http.ResponseWriter, r *http.Request) {
	var vReq VertexRequest
	if err := json.NewDecoder(r.Body).Decode(&vReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Convert to Provider format
	if api.provider == "ollama" {
		api.proxyToOllama(w, vReq)
	} else if api.provider == "openai" {
		api.proxyToOpenAI(w, vReq)
	} else {
		http.Error(w, "Unsupported AI provider", http.StatusInternalServerError)
	}
}

func (api *API) proxyToOllama(w http.ResponseWriter, vReq VertexRequest) {
	// Ollama /api/chat format
	type OllamaMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type OllamaReq struct {
		Model    string      `json:"model"`
		Messages []OllamaMsg `json:"messages"`
		Stream   bool        `json:"stream"`
	}

	oReq := OllamaReq{
		Model:  api.model,
		Stream: false,
	}
	for _, c := range vReq.Contents {
		role := c.Role
		if role == "" { role = "user" }
		var texts []string
		for _, p := range c.Parts { texts = append(texts, p.Text) }
		oReq.Messages = append(oReq.Messages, OllamaMsg{
			Role:    role,
			Content: strings.Join(texts, "\n"),
		})
	}

	body, _ := json.Marshal(oReq)
	resp, err := http.Post(api.endpoint+"/api/chat", "application/json", bytes.NewBuffer(body))
	if err != nil {
		http.Error(w, "Ollama connection failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		io.Copy(w, resp.Body)
		return
	}

	var oResp struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	}
	json.NewDecoder(resp.Body).Decode(&oResp)

	// Convert back to Vertex
	vResp := VertexResponse{}
	vResp.Candidates = make([]struct {
		Content struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	}, 1)
	vResp.Candidates[0].Content.Role = "model"
	vResp.Candidates[0].Content.Parts = append(vResp.Candidates[0].Content.Parts, struct {
		Text string `json:"text"`
	}{Text: oResp.Message.Content})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vResp)
}

func (api *API) handleListModels(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	provider := api.provider
	endpoint := api.endpoint
	api.mu.Unlock()

	var models []string

	if provider == "ollama" {
		resp, err := http.Get(endpoint + "/api/tags")
		if err == nil {
			defer resp.Body.Close()
			var oResp struct {
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}
			json.NewDecoder(resp.Body).Decode(&oResp)
			for _, m := range oResp.Models {
				models = append(models, m.Name)
			}
		}
	} else if provider == "openai" {
		// Mocked OpenAI/OpenRouter model discovery
		models = append(models, "gpt-3.5-turbo", "gpt-4", "claude-3-opus", "meta-llama/llama-3-70b")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"models": models,
	})
}

func (api *API) proxyToOpenAI(w http.ResponseWriter, vReq VertexRequest) {
	// Similar translation for OpenAI-compatible APIs (OpenRouter, local llama.cpp)
	// For brevity, using a similar structure
	log.Printf("[Shim: Vertex AI] Proxying to OpenAI endpoint: %s", api.endpoint)
	w.WriteHeader(http.StatusNotImplemented)
}
