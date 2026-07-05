# GSoC 2025 Organizations Scraper

This scraper collects data from:
- `https://summerofcode.withgoogle.com/programs/2025/organizations`

It outputs JSON with:
1. Each organization's GitHub URLs discovered on that org's **View Idea List** page.
2. Each organization's overview (`description` from GSoC organizations API).
3. Each organization's project overviews (from GSoC projects API).

## Files

- `scrape_gsoc.py` – scraper script
- `output/gsoc_2025_organizations_projects.json` – generated result

## Requirements

- Python 3.9+
- No third-party dependencies required

## Run

From this folder:

```powershell
python .\scrape_gsoc.py --program 2025 --output-dir .\output
```

Go version:

```powershell
go run .\scrape_gsoc.go --program 2025 --output-dir .\output
```

Optional flags:

```powershell
python .\scrape_gsoc.py --program 2025 --output-dir .\output --workers 12
```

```powershell
go run .\scrape_gsoc.go --program 2025 --output-dir .\output --workers 12
```

## JSON schema (high-level)

Top-level:
- `program`
- `source_page`
- `organizations_count`
- `organizations[]`

Each organization includes:
- `name`
- `slug`
- `website_url`
- `ideas_link`
- `source_code`
- `org_overview`
- `github_urls_from_idea_list_page[]`
- `projects_overview[]`
- `project_count`
- `ideas_scrape_error`

Each item in `projects_overview[]` includes:
- `title`
- `overview`
- `size`
- `status`
- `contributor_name`
- `project_code_url`
- `organization_slug`

## Notes

- Some idea-list links are non-GitHub pages (docs, GitLab, websites). In those cases, `github_urls_from_idea_list_page` may be empty.
- If an idea-list page blocks or fails, that org gets an `ideas_scrape_error` message while the scraper continues.
