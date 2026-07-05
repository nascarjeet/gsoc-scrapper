#!/usr/bin/env python3
import argparse
import json
import re
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Any
from urllib.parse import urlencode
from urllib.request import Request, urlopen

BASE_URL = "https://summerofcode.withgoogle.com"
UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GSoC-Scraper/1.0"


def fetch_json(url: str, timeout: int = 20) -> Any:
    req = Request(url, headers={"User-Agent": UA, "Accept": "application/json"})
    with urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode("utf-8"))


def fetch_text(url: str, timeout: int = 20) -> str:
    req = Request(url, headers={"User-Agent": UA})
    with urlopen(req, timeout=timeout) as resp:
        return resp.read().decode("utf-8", errors="replace")


def norm_url(url: str) -> str:
    return url.rstrip(").,;:'\"")


def extract_github_urls(text: str) -> list[str]:
    matches = re.findall(r"https?://(?:www\.)?github\.com/[^\s\"'<>]+", text, flags=re.IGNORECASE)
    deduped = []
    seen = set()
    for raw in matches:
        cleaned = norm_url(raw)
        key = cleaned.lower()
        if key not in seen:
            seen.add(key)
            deduped.append(cleaned)
    return deduped


def org_projects(program_year: str, organization_slug: str) -> list[dict[str, Any]]:
    query = urlencode(
        {
            "role": "",
            "program_slug": program_year,
            "organization_slug": organization_slug,
        }
    )
    url = f"{BASE_URL}/api/projects/?{query}"
    payload = fetch_json(url)
    entities = payload.get("entities", {})
    projects = entities.get("projects", [])
    if isinstance(projects, list):
        return projects
    return []


def summarize_projects(projects: list[dict[str, Any]]) -> list[dict[str, Any]]:
    summarized: list[dict[str, Any]] = []
    for p in projects:
        summarized.append(
            {
                "title": p.get("title"),
                "overview": p.get("body_short") or p.get("body"),
                "size": p.get("size"),
                "status": p.get("status"),
                "contributor_name": p.get("contributor_name"),
                "project_code_url": p.get("project_code_url"),
                "organization_slug": p.get("organization_slug"),
            }
        )
    return summarized


def scrape_one_org(program_year: str, org: dict[str, Any]) -> dict[str, Any]:
    slug = org.get("slug")
    ideas_link = org.get("ideas_link")
    source_code = org.get("source_code")

    github_from_ideas: list[str] = []
    ideas_scrape_error = None
    if ideas_link:
        try:
            ideas_html = fetch_text(ideas_link)
            github_from_ideas = extract_github_urls(ideas_html)
        except Exception as exc:  # narrow surface: report per-org failure, continue overall scrape
            ideas_scrape_error = str(exc)

    projects = org_projects(program_year, slug)

    return {
        "name": org.get("name"),
        "slug": slug,
        "website_url": org.get("website_url"),
        "ideas_link": ideas_link,
        "source_code": source_code,
        "org_overview": org.get("description"),
        "github_urls_from_idea_list_page": github_from_ideas,
        "projects_overview": summarize_projects(projects),
        "project_count": len(projects),
        "ideas_scrape_error": ideas_scrape_error,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Scrape GSoC organizations, GitHub links, and project overviews.")
    parser.add_argument("--program", default="2025", help="Program year slug (default: 2025)")
    parser.add_argument("--output-dir", default="output", help="Directory for JSON output")
    parser.add_argument("--workers", type=int, default=10, help="Parallel workers for org scraping")
    args = parser.parse_args()

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    orgs_url = f"{BASE_URL}/api/program/{args.program}/organizations/"
    organizations: list[dict[str, Any]] = fetch_json(orgs_url)

    items: list[dict[str, Any]] = []
    with ThreadPoolExecutor(max_workers=max(1, args.workers)) as pool:
        futures = [pool.submit(scrape_one_org, args.program, org) for org in organizations]
        for fut in as_completed(futures):
            items.append(fut.result())

    items.sort(key=lambda x: (x.get("name") or "").lower())

    payload = {
        "program": args.program,
        "source_page": f"{BASE_URL}/programs/{args.program}/organizations",
        "organizations_count": len(items),
        "organizations": items,
    }

    out_file = output_dir / f"gsoc_{args.program}_organizations_projects.json"
    out_file.write_text(json.dumps(payload, indent=2, ensure_ascii=False), encoding="utf-8")
    print(f"Wrote: {out_file.resolve()}")


if __name__ == "__main__":
    main()
