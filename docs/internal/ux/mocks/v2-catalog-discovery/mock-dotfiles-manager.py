#!/usr/bin/env python3
"""Runnable UX mock for the recontracted #228 catalog discovery surface.

This mock is intentionally separate from product code. It simulates the accepted
official-catalog discovery target from
docs/internal/ux/v2-catalog-discovery-storyboard.md.

Safety: the mock never reads or writes live app settings, the user's real
settings storage folder, real catalog folders, secrets, or network resources.
"""
from __future__ import annotations

import sys

OFFICIAL_APPS = ["git", "nvim", "ssh", "starship", "tmux", "zsh"]
OFFICIAL_CATALOG_VERSION = "9f2c7a1"
OFFICIAL_CATALOG_UPDATED = "2026-06-30 18:00 UTC"


def out(text: str = "") -> None:
    print(text)


def print_list() -> int:
    out("Supported apps")
    out("")
    out("  APP       CATALOG   STATE")
    for app in OFFICIAL_APPS:
        out(f"  {app:<8}  official  not managed")
    out("")
    out("Use `dotfiles-manager explain <app>` to see what can be managed.")
    return 0


def print_search(args: list[str]) -> int:
    query = args[0] if args else ""
    if query.lower() == "git":
        out('Search results for "git"')
        out("")
        out("  APP  CATALOG   STATE")
        out("  git  official  not managed")
        out("")
        out("Use `dotfiles-manager explain git` to see what can be managed.")
        return 0
    out(f'No supported apps found for "{query}".')
    out("")
    out("The current official catalog supports:")
    out("  git, nvim, ssh, starship, tmux, zsh")
    out("")
    out("This version searches only the current official catalog.")
    out("Future versions may refresh official support data or add remote catalogs.")
    out("This version cannot do that yet.")
    return 0


def print_explain(args: list[str]) -> int:
    app = args[0] if args else ""
    if app == "git":
        out("Git is supported.")
        out("")
        out("App ID: git")
        out("Catalog: official")
        out("State: not managed")
        out("")
        out("Can manage:")
        out("  git:user.email  Git user email")
        out("  git:user.name   Git user name")
        out("")
        out("Does not manage:")
        out("  credential.helper")
        out("  [credential] sections")
        out("  include/includeIf expansion")
        return 0
    out(f"App not supported: {app}")
    return 1


def catalog_list() -> int:
    out("Catalogs")
    out("")
    out("Catalogs define app/tool support; they do not store your settings.")
    out("")
    out("  dotfiles-manager/official  active for discovery")
    out(f"    Catalog version: {OFFICIAL_CATALOG_VERSION}")
    out(f"    Catalog updated: {OFFICIAL_CATALOG_UPDATED}")
    return 0


def main(argv: list[str]) -> int:
    if not argv:
        out("dotfiles-manager UX mock: missing command")
        return 1
    cmd, rest = argv[0], argv[1:]
    if cmd == "list":
        return print_list()
    if cmd == "search":
        return print_search(rest)
    if cmd == "explain":
        return print_explain(rest)
    if cmd == "catalog":
        if not rest:
            out("catalog: missing subcommand")
            return 1
        sub, subargs = rest[0], rest[1:]
        if sub == "list":
            return catalog_list()
    out(f"dotfiles-manager UX mock: unsupported command {' '.join(argv)!r}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
