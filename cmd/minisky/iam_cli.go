package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var iamCmd = &cobra.Command{
	Use:   "iam",
	Short: "Inspect the IAM policies on the resource hierarchy",
	Long: "Answers what a policy grants, following inheritance down the\n" +
		"organization, folder and project hierarchy.\n\n" +
		"MiniSky records policies but does not enforce them — it authenticates\n" +
		"nobody — so these answers describe the configuration, not whether a\n" +
		"request would be allowed against Google.",
}

func init() {
	var principal, resource, permission string

	explain := &cobra.Command{
		Use:   "explain",
		Short: "Show what a principal holds on a resource, and where it came from",
		Example: "  minisky iam explain --principal user:ada@example.com --resource projects/my-app\n" +
			"  minisky iam explain --principal user:ada@example.com --resource folders/123 \\\n" +
			"      --permission compute.instances.create\n" +
			"  minisky iam explain --resource projects/my-app --permission storage.objects.delete",
		Run: func(cmd *cobra.Command, args []string) {
			if resource == "" {
				fmt.Fprintln(os.Stderr, "--resource is required, as organizations/{id}, folders/{id} or projects/{id}")
				os.Exit(1)
			}
			if principal == "" && permission == "" {
				fmt.Fprintln(os.Stderr, "give --principal, --permission, or both")
				os.Exit(1)
			}
			runExplain(principal, resource, permission)
		},
	}

	explain.Flags().StringVar(&principal, "principal", "", "principal, e.g. user:ada@example.com or serviceAccount:ci@example.com")
	explain.Flags().StringVar(&resource, "resource", "", "resource to inspect")
	explain.Flags().StringVar(&permission, "permission", "", "permission to check, e.g. compute.instances.create")

	iamCmd.AddCommand(explain)
	rootCmd.AddCommand(iamCmd)
}

type explainResponse struct {
	Principal string   `json:"principal"`
	Resource  string   `json:"resource"`
	Ancestry  []string `json:"ancestry"`
	Roles     []struct {
		Role      string `json:"role"`
		GrantedOn string `json:"grantedOn"`
		Inherited bool   `json:"inherited"`
		RoleKnown bool   `json:"roleKnown"`
	} `json:"roles"`
	Permission  string   `json:"permission"`
	Granted     *bool    `json:"granted"`
	GrantedBy   []string `json:"grantedBy"`
	UnknownRole []string `json:"rolesNotInCatalogue"`
	Caveat      string   `json:"caveat"`

	// Returned when only a permission was asked about.
	Principals []string `json:"principals"`
}

func runExplain(principal, resource, permission string) {
	payload, _ := json.Marshal(map[string]string{
		"principal": principal, "resource": resource, "permission": permission,
	})

	port := os.Getenv("MINISKY_PORT")
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("http://localhost:"+port+"/v1/internal/iam/analyze",
		"application/json", bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not reach MiniSky on :%s — is the daemon running? (%v)\n", port, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var out explainResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		fmt.Fprintf(os.Stderr, "unexpected response: %v\n", err)
		os.Exit(1)
	}

	// "Who can do this here" — the question a review starts from.
	if principal == "" {
		fmt.Printf("Principals with %s on %s:\n", permission, resource)
		if len(out.Principals) == 0 {
			fmt.Println("  (none)")
			return
		}
		for _, p := range out.Principals {
			fmt.Println("  " + p)
		}
		return
	}

	fmt.Printf("%s on %s\n\n", out.Principal, out.Resource)

	if len(out.Ancestry) > 1 {
		fmt.Println("Inherits from:")
		for _, a := range out.Ancestry[1:] {
			fmt.Println("  " + a)
		}
		fmt.Println()
	}

	if len(out.Roles) == 0 {
		fmt.Println("Holds no roles here.")
	} else {
		fmt.Println("Roles held:")
		for _, r := range out.Roles {
			source := "granted directly"
			if r.Inherited {
				source = "inherited from " + r.GrantedOn
			}
			note := ""
			if !r.RoleKnown {
				note = "   [not in the role catalogue]"
			}
			fmt.Printf("  %-46s %s%s\n", r.Role, source, note)
		}
	}

	if out.Permission != "" {
		fmt.Println()
		verdict := "NO"
		if out.Granted != nil && *out.Granted {
			verdict = "YES"
		}
		fmt.Printf("%s  %s\n", verdict, out.Permission)
		for _, by := range out.GrantedBy {
			fmt.Println("      via " + by)
		}
	}

	if out.Caveat != "" {
		fmt.Println("\nNote: " + out.Caveat)
	}
}
