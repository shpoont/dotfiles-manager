#!/usr/bin/env python3
"""Runnable UX mock for dotfiles-manager v2 catalog discovery (#228).

This mock is intentionally separate from product code. It simulates the accepted
CLI surface from docs/internal/ux/v2-catalog-discovery-storyboard.md so reviewers
can run the intended journey before relying on the runtime implementation.

Safety: the mock never reads or writes live app settings, stored settings, or
real catalog contents. Its only optional write is a JSON state file selected by
DOTFILES_MANAGER_UX_MOCK_STATE, normally under a temporary directory created by
run-demo.sh.
"""
from __future__ import annotations

import json
import os
import sys
from pathlib import Path
from typing import Any

BUILT_IN_APPS = ["custom.files", "git", "nvim", "ssh", "starship", "tmux", "zsh"]
LOCAL_APPS = ["example-tool", "git"]
DEFAULT_STATE: dict[str, Any] = {
    "catalogs": {},
    "removed_catalogs": [],
}


def state_path() -> Path | None:
    raw = os.environ.get("DOTFILES_MANAGER_UX_MOCK_STATE")
    if not raw:
        return None
    return Path(raw)


def load_state() -> dict[str, Any]:
    path = state_path()
    if not path or not path.exists():
        return json.loads(json.dumps(DEFAULT_STATE))
    with path.open("r", encoding="utf-8") as f:
        loaded = json.load(f)
    state = json.loads(json.dumps(DEFAULT_STATE))
    state.update(loaded)
    state.setdefault("catalogs", {})
    state.setdefault("removed_catalogs", [])
    return state


def save_state(state: dict[str, Any]) -> None:
    path = state_path()
    if not path:
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as f:
        json.dump(state, f, indent=2, sort_keys=True)
        f.write("\n")


def has_enabled_personal(state: dict[str, Any]) -> bool:
    personal = state.get("catalogs", {}).get("personal")
    return bool(personal and personal.get("enabled"))


def has_disabled_personal(state: dict[str, Any]) -> bool:
    personal = state.get("catalogs", {}).get("personal")
    return bool(personal and not personal.get("enabled"))


def ever_had_personal(state: dict[str, Any]) -> bool:
    return "personal" in state.get("catalogs", {}) or "personal" in state.get("removed_catalogs", [])


def out(text: str = "") -> None:
    print(text)


def remoteish(value: str) -> bool:
    if "://" in value:
        return True
    if value.startswith(("/", "./", "../", "~")):
        return False
    return value.count("/") == 1


def print_list(state: dict[str, Any]) -> int:
    enabled = has_enabled_personal(state)
    disabled = has_disabled_personal(state)
    out("Supported apps")
    out("")
    out("  APP           SOURCE              STATE" if enabled else "  APP           SOURCE    STATE")
    if enabled:
        out("  custom.files  built-in            not managed")
        out("  example-tool  personal            not managed")
        out("  git           built-in            not managed")
        out("                also in personal; built-in remains default")
        out("  nvim          built-in            not managed")
        out("  ssh           built-in            not managed")
        out("  starship      built-in            not managed")
        out("  tmux          built-in            not managed")
        out("  zsh           built-in            not managed")
        out("")
        out("Use `dotfiles-manager explain <app>` to see support details and candidates.")
    else:
        for app in BUILT_IN_APPS:
            out(f"  {app:<12}  built-in  not managed")
        if disabled:
            out("")
            out("Disabled local catalogs:")
            out("  personal  2 hidden apps/candidates")
        out("")
        out("Tip: add a local catalog when you want dotfiles-manager to discover custom app support.")
        out("     dotfiles-manager catalog add ~/dotfiles-manager-recipes --name personal")
    out("No live settings were read or changed.")
    out("No stored settings were changed.")
    return 0


def print_search(args: list[str], state: dict[str, Any]) -> int:
    query = args[0] if args else ""
    out(f"Search results for {query!r}")
    out("")
    if query.lower() in {"git", "gi"}:
        out("  git  built-in  not managed")
        if has_enabled_personal(state):
            out("       also in personal; built-in remains default")
        out("")
        out("Use `dotfiles-manager explain git` to see what can be managed.")
    elif has_enabled_personal(state) and query.lower() in {"example", "example-tool", "tool"}:
        out("  example-tool  personal  not managed")
        out("")
        out("Use `dotfiles-manager explain example-tool` to see what can be managed.")
    else:
        out("  No supported apps matched.")
    out("No live settings were read or changed.")
    return 0


def print_explain(args: list[str], state: dict[str, Any]) -> int:
    app = args[0] if args else ""
    if app == "git" and has_enabled_personal(state):
        out("Git is supported by multiple sources.")
        out("")
        out("Default source:")
        out("  built-in support from dotfiles-manager")
        out("")
        out("Other available source:")
        out("  local catalog: personal")
        out("  Status: candidate only")
        out("")
        out("Why built-in is used:")
        out("  Built-in support remains the default unless you explicitly choose another")
        out("  source. Local support cannot silently replace built-in support.")
        out("")
        out("Can manage from the default source:")
        out("  git:user.email  Git user email")
        out("  git:user.name   Git user name")
        out("")
        out("No live values were printed.")
        out("No live settings were changed.")
        return 0
    if app == "git":
        out("Git is supported by dotfiles-manager.")
        out("")
        out("App ID: git")
        out("Source: built-in support from dotfiles-manager")
        out("State: not managed")
        out("")
        out("Can manage:")
        out("  git:user.email  Git user email")
        out("  git:user.name   Git user name")
        out("")
        out("Why this source is used:")
        out("  Built-in support is bundled, deterministic, and updates with each")
        out("  dotfiles-manager release.")
        out("")
        out("No live values were printed.")
        out("No live settings were changed.")
        return 0
    if app == "example-tool" and has_enabled_personal(state):
        out("Example Tool is supported by a local catalog.")
        out("")
        out("App ID: example-tool")
        out("Source: local catalog personal")
        out("State: not managed")
        out("")
        out("Can manage:")
        out("  example-tool:config  Config file")
        out("    Live location: $HOME/.config/example-tool/config.yaml")
        out("")
        out("Before live settings can be changed:")
        out("  dotfiles-manager will show this source and the paths it wants to manage.")
        out("  Local support requires write approval before it can change live settings.")
        out("")
        out("No live values were printed.")
        out("No live settings were changed.")
        return 0
    out(f"{app or '<missing app>'}: not supported by enabled catalogs")
    out("No live settings were read or changed.")
    return 1


def catalog_list(state: dict[str, Any]) -> int:
    out("Catalogs")
    out("")
    out("  built-in  enabled")
    out("    Source: ships with dotfiles-manager")
    out("    Updates: with dotfiles-manager releases")
    out("    Network: not used")
    out("    Removable: no")
    catalogs = state.get("catalogs", {})
    if catalogs:
        out("")
        for name, info in sorted(catalogs.items()):
            status = "enabled" if info.get("enabled") else "disabled"
            out(f"  {name}  {status}  local catalog")
            out(f"    Source: {info.get('path')}")
            out("    Support: 2 valid apps/candidates, 0 blocked")
            out("    Network: not used")
    else:
        out("")
        out("Local catalogs: none")
    out("Remote catalogs: not supported yet")
    out("")
    out("No live settings were read or changed.")
    out("No stored settings were changed.")
    return 0


def catalog_add(args: list[str], state: dict[str, Any]) -> int:
    if not args:
        out("Catalog not added: missing path")
        return 1
    path = args[0]
    name = None
    if "--name" in args:
        idx = args.index("--name")
        if idx + 1 < len(args):
            name = args[idx + 1]
    display = name or path
    if remoteish(path):
        out(f"Catalog not added: {display}")
        out("")
        out("Reason:")
        out("  Remote GitHub catalogs are not supported in this version of dotfiles-manager.")
        out("")
        out("For now, use a local catalog folder:")
        out("  dotfiles-manager catalog add ./custom-recipes --name personal")
        out("")
        out("Remote catalog trust, updates, and write gates are planned separately.")
        out("No live settings were read or changed.")
        out("No stored settings were changed.")
        return 1
    if not name:
        out(f"Catalog not added: {path}")
        out("")
        out("Reason:")
        out("  Local catalog name is required in this mock.")
        out("")
        out("Use:")
        out("  dotfiles-manager catalog add <local-path> --name <name>")
        return 1
    if "broken-recipes" in path:
        out(f"Catalog not added: {name}")
        out("")
        out("Reason:")
        out("  1 support definition failed validation.")
        out("")
        out("Invalid support:")
        out("  broken-tool")
        out('    - unknown field "dangerousCommand"')
        out("")
        out("No live settings were read or changed.")
        out("No stored settings were changed.")
        return 1
    state["catalogs"][name] = {"path": path, "enabled": True}
    if name in state.get("removed_catalogs", []):
        state["removed_catalogs"] = [x for x in state["removed_catalogs"] if x != name]
    save_state(state)
    out(f"Added local catalog: {name}")
    out("")
    out("Source:")
    out(f"  {path}")
    out("")
    out("Validated support:")
    out("  example-tool  local support")
    out("  git           local candidate; built-in support remains the default")
    out("")
    out("Network: not used")
    out("No live settings were read or changed.")
    out("No stored settings were changed.")
    return 0


def catalog_disable(args: list[str], state: dict[str, Any]) -> int:
    name = args[0] if args else ""
    if name not in state.get("catalogs", {}):
        out(f"Catalog not disabled: {name}")
        out("Reason: local catalog is not known to dotfiles-manager.")
        return 1
    state["catalogs"][name]["enabled"] = False
    save_state(state)
    out(f"Disabled local catalog: {name}")
    out("")
    out("No longer available from this catalog:")
    out("  example-tool")
    out("  git local candidate")
    out("")
    out("Nothing was deleted.")
    out("Live app settings were not changed.")
    out("Stored settings were not changed.")
    out("")
    out("If a managed app depends on this catalog, it will show as source unavailable")
    out("until you enable the catalog, add another source, or stop managing that app.")
    return 0


def catalog_enable(args: list[str], state: dict[str, Any]) -> int:
    name = args[0] if args else ""
    if name not in state.get("catalogs", {}):
        out(f"Catalog not enabled: {name}")
        out("Reason: local catalog is not known to dotfiles-manager.")
        return 1
    state["catalogs"][name]["enabled"] = True
    save_state(state)
    out(f"Enabled local catalog: {name}")
    out("")
    out("Validated support:")
    out("  example-tool  local support")
    out("  git           local candidate; built-in support remains the default")
    out("")
    out("No live settings were read or changed.")
    out("No stored settings were changed.")
    return 0


def catalog_remove(args: list[str], state: dict[str, Any]) -> int:
    name = args[0] if args else ""
    catalogs = state.get("catalogs", {})
    if name not in catalogs:
        out(f"Catalog not removed: {name}")
        out("Reason: local catalog is not known to dotfiles-manager.")
        return 1
    path = catalogs[name].get("path", "<unknown>")
    del catalogs[name]
    state.setdefault("removed_catalogs", [])
    if name not in state["removed_catalogs"]:
        state["removed_catalogs"].append(name)
    save_state(state)
    out(f"Removed local catalog: {name}")
    out("")
    out("Forgotten by dotfiles-manager:")
    out(f"  {path}")
    out("")
    out("Nothing was deleted from that folder.")
    out("Live app settings were not changed.")
    out("Stored settings were not changed.")
    out("")
    out("Apps that depended on this catalog are now source unavailable until you re-add")
    out("this catalog, choose another source, or stop managing those apps.")
    return 0


def print_status(args: list[str], state: dict[str, Any]) -> int:
    app = args[0] if args else ""
    if app == "example-tool" and (not has_enabled_personal(state)) and ever_had_personal(state):
        out("example-tool: blocked")
        out("")
        out("Reason:")
        out('  This app is managed with support from local catalog "personal", but that')
        out("  catalog is disabled or removed.")
        out("")
        out("No live app settings were read or changed.")
        out("Stored settings were not changed.")
        out("Stored settings still exist, if they existed before.")
        out("")
        out("To continue:")
        out("  Enable the catalog:")
        out("    dotfiles-manager catalog enable personal")
        out("")
        out("  Or add another catalog that supports example-tool.")
        out("")
        out("  Or remove this app from the managed set.")
        return 0
    out(f"{app or 'all apps'}: no changes detected by this mock")
    out("No live settings were read or changed.")
    out("No stored settings were changed.")
    return 0


def print_sync(args: list[str], state: dict[str, Any]) -> int:
    app_args = [a for a in args if not a.startswith("--")]
    app = app_args[0] if app_args else ""
    if app == "example-tool" and has_enabled_personal(state):
        out("Preview sync for example-tool")
        out("")
        out("Source:")
        out("  local catalog personal")
        out("")
        out("This support wants to manage:")
        out("  example-tool:config")
        out("    Live location: $HOME/.config/example-tool/config.yaml")
        out("    Stored settings: desired/user/<user-id>/targets/example-tool/...")
        out("")
        out("Result:")
        out("  Blocked before write.")
        out("")
        out("Reason:")
        out("  Local support requires write approval before dotfiles-manager can change live")
        out("  settings for this app.")
        out("")
        out("Next step:")
        out("  Review and approve this local support before allowing writes.")
        out("  If this version does not include an approval command yet, this app remains")
        out("  blocked for live writes.")
        out("")
        out("No live settings were changed.")
        out("No stored settings were changed.")
        return 0
    return print_status([app], state)


def main(argv: list[str]) -> int:
    if not argv:
        out("dotfiles-manager UX mock: missing command")
        return 1
    state = load_state()
    cmd, rest = argv[0], argv[1:]
    if cmd == "list":
        return print_list(state)
    if cmd == "search":
        return print_search(rest, state)
    if cmd == "explain":
        return print_explain(rest, state)
    if cmd == "status":
        return print_status([a for a in rest if not a.startswith("--")], state)
    if cmd == "sync":
        return print_sync(rest, state)
    if cmd == "catalog":
        if not rest:
            out("catalog: missing subcommand")
            return 1
        sub, subargs = rest[0], rest[1:]
        if sub == "list":
            return catalog_list(state)
        if sub == "add":
            return catalog_add(subargs, state)
        if sub == "disable":
            return catalog_disable(subargs, state)
        if sub == "enable":
            return catalog_enable(subargs, state)
        if sub == "remove":
            return catalog_remove(subargs, state)
    out(f"dotfiles-manager UX mock: unsupported command {' '.join(argv)!r}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
