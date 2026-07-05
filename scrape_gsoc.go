package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	baseURL   = "https://summerofcode.withgoogle.com"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GSoC-Go-Scraper/1.0"
)

type OrganizationAPI struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	WebsiteURL  string `json:"website_url"`
	IdeasLink   string `json:"ideas_link"`
	SourceCode  string `json:"source_code"`
	Description string `json:"description"`
}

type ProjectAPI struct {
	Title            string `json:"title"`
	BodyShort        string `json:"body_short"`
	Body             string `json:"body"`
	Size             string `json:"size"`
	Status           string `json:"status"`
	ContributorName  string `json:"contributor_name"`
	ProjectCodeURL   string `json:"project_code_url"`
	OrganizationSlug string `json:"organization_slug"`
}

type ProjectsEnvelope struct {
	Entities struct {
		Projects []ProjectAPI `json:"projects"`
	} `json:"entities"`
}

type ProjectSummary struct {
	Title            string `json:"title"`
	Overview         string `json:"overview"`
	Size             string `json:"size"`
	Status           string `json:"status"`
	ContributorName  string `json:"contributor_name"`
	ProjectCodeURL   string `json:"project_code_url"`
	OrganizationSlug string `json:"organization_slug"`
}

type OrgResult struct {
	Name                    string           `json:"name"`
	Slug                    string           `json:"slug"`
	WebsiteURL              string           `json:"website_url"`
	IdeasLink               string           `json:"ideas_link"`
	SourceCode              string           `json:"source_code"`
	OrgOverview             string           `json:"org_overview"`
	GitHubURLsFromIdeaList  []string         `json:"github_urls_from_idea_list_page"`
	ProjectsOverview        []ProjectSummary `json:"projects_overview"`
	ProjectCount            int              `json:"project_count"`
	IdeasScrapeError        *string          `json:"ideas_scrape_error"`
}

type OutputPayload struct {
	Program            string      `json:"program"`
	SourcePage         string      `json:"source_page"`
	OrganizationsCount int         `json:"organizations_count"`
	Organizations      []OrgResult `json:"organizations"`
}

var githubURLRegex = regexp.MustCompile(`https?://(?:www\.)?github\.com/[^\s"'<>]+`)

func fetchJSON(client *http.Client, u string, out any) error {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request failed %s: status %d", u, resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func fetchText(client *http.Client, u string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("request failed %s: status %d", u, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func normalizeURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), ").,;:'\"")
}

func extractGitHubURLs(text string) []string {
	matches := githubURLRegex.FindAllString(text, -1)
	seen := make(map[string]struct{})
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		clean := normalizeURL(m)
		k := strings.ToLower(clean)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func orgProjects(client *http.Client, programYear, orgSlug string) ([]ProjectAPI, error) {
	q := url.Values{}
	q.Set("role", "")
	q.Set("program_slug", programYear)
	q.Set("organization_slug", orgSlug)
	u := fmt.Sprintf("%s/api/projects/?%s", baseURL, q.Encode())

	var env ProjectsEnvelope
	if err := fetchJSON(client, u, &env); err != nil {
		return nil, err
	}
	return env.Entities.Projects, nil
}

func summarizeProjects(projects []ProjectAPI) []ProjectSummary {
	out := make([]ProjectSummary, 0, len(projects))
	for _, p := range projects {
		overview := p.BodyShort
		if overview == "" {
			overview = p.Body
		}
		out = append(out, ProjectSummary{
			Title:            p.Title,
			Overview:         overview,
			Size:             p.Size,
			Status:           p.Status,
			ContributorName:  p.ContributorName,
			ProjectCodeURL:   p.ProjectCodeURL,
			OrganizationSlug: p.OrganizationSlug,
		})
	}
	return out
}

func scrapeOneOrg(client *http.Client, programYear string, org OrganizationAPI) OrgResult {
	var githubFromIdeas []string
	var scrapeErr *string

	if org.IdeasLink != "" {
		ideasHTML, err := fetchText(client, org.IdeasLink)
		if err != nil {
			s := err.Error()
			scrapeErr = &s
		} else {
			githubFromIdeas = extractGitHubURLs(ideasHTML)
		}
	}

	projects, err := orgProjects(client, programYear, org.Slug)
	if err != nil {
		empty := []ProjectSummary{}
		s := err.Error()
		if scrapeErr == nil {
			scrapeErr = &s
		}
		return OrgResult{
			Name:                   org.Name,
			Slug:                   org.Slug,
			WebsiteURL:             org.WebsiteURL,
			IdeasLink:              org.IdeasLink,
			SourceCode:             org.SourceCode,
			OrgOverview:            org.Description,
			GitHubURLsFromIdeaList: githubFromIdeas,
			ProjectsOverview:       empty,
			ProjectCount:           0,
			IdeasScrapeError:       scrapeErr,
		}
	}

	projectSummaries := summarizeProjects(projects)
	return OrgResult{
		Name:                   org.Name,
		Slug:                   org.Slug,
		WebsiteURL:             org.WebsiteURL,
		IdeasLink:              org.IdeasLink,
		SourceCode:             org.SourceCode,
		OrgOverview:            org.Description,
		GitHubURLsFromIdeaList: githubFromIdeas,
		ProjectsOverview:       projectSummaries,
		ProjectCount:           len(projectSummaries),
		IdeasScrapeError:       scrapeErr,
	}
}

func main() {
	program := flag.String("program", "2025", "Program year slug")
	outputDir := flag.String("output-dir", "output", "Directory for JSON output")
	workers := flag.Int("workers", 10, "Parallel workers for org scraping")
	flag.Parse()

	if *workers < 1 {
		*workers = 1
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output dir: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 25 * time.Second}

	orgsURL := fmt.Sprintf("%s/api/program/%s/organizations/", baseURL, *program)
	var organizations []OrganizationAPI
	if err := fetchJSON(client, orgsURL, &organizations); err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch organizations: %v\n", err)
		os.Exit(1)
	}

	jobs := make(chan OrganizationAPI)
	resultsCh := make(chan OrgResult)
	var wg sync.WaitGroup

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for org := range jobs {
				resultsCh <- scrapeOneOrg(client, *program, org)
			}
		}()
	}

	go func() {
		for _, org := range organizations {
			jobs <- org
		}
		close(jobs)
		wg.Wait()
		close(resultsCh)
	}()

	results := make([]OrgResult, 0, len(organizations))
	for r := range resultsCh {
		results = append(results, r)
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})

	payload := OutputPayload{
		Program:            *program,
		SourcePage:         fmt.Sprintf("%s/programs/%s/organizations", baseURL, *program),
		OrganizationsCount: len(results),
		Organizations:      results,
	}

	outFile := filepath.Join(*outputDir, fmt.Sprintf("gsoc_%s_organizations_projects.json", *program))
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode output json: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outFile, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote: %s\n", outFile)
}

