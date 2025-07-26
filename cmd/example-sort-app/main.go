package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"time"

	client_sdk "github.com/goriiin/go-ab-service/pkg/client-sdk"
)

const (
	VariantAsc  = "variant-a-asc"
	VariantDesc = "variant-b-desc"
)

type SortRequest struct {
	UserID  string `json:"user_id"`
	Numbers []int  `json:"numbers"`
}

type SortResponse struct {
	SortedNumbers []int  `json:"sorted_numbers"`
	VariantUsed   string `json:"variant_used"`
	Source        string `json:"source"`
}

func main() {
	sdkConfig := client_sdk.Config{
		RelevantLayerIDs:      []string{"sorting_layer"},
		KafkaBrokers:          []string{"kafka:9092"},
		KafkaGroupID:          "sort-app-sdk-group",
		MinIOEndpoint:         "minio:9000",
		MinIOAccessKey:        "minioadmin",
		MinIOSecretKey:        "minioadmin",
		MinIOUseSSL:           false,
		SnapshotBucket:        "ab-snapshots",
		LocalCachePath:        "/tmp/ab_cache.json",
		LocalCacheTTL:         24 * time.Hour,
		AssignmentEventsTopic: "ab_assignment_events",
	}

	initCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	abClient, err := client_sdk.NewClient(initCtx, sdkConfig)
	if err != nil {
		log.Fatalf("FATAL: Failed to initialize A/B client: %v", err)
	}
	defer abClient.Close()

	http.HandleFunc("/sort", sortHandler(abClient))
	log.Println("INFO: Server is listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

func sortHandler(abClient *client_sdk.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SortRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		attributes := map[string]any{"use_sort_test": true}
		assignments := abClient.Decide(req.UserID, attributes)

		variant := ""
		if len(assignments) > 0 {
			for _, v := range assignments {
				variant = v
				break
			}
		}

		resp := SortResponse{
			SortedNumbers: req.Numbers,
		}

		if variant != "" {
			resp.Source = "A/B Experiment"
			resp.VariantUsed = variant
			if variant == VariantDesc {
				sort.Sort(sort.Reverse(sort.IntSlice(resp.SortedNumbers)))
			} else {
				sort.Ints(resp.SortedNumbers)
			}
		} else {
			resp.Source = "Default"
			resp.VariantUsed = "default"
			sort.Ints(resp.SortedNumbers)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
