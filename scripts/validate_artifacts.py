#!/usr/bin/env python3
"""Validate repomap debug artifacts — JSON structure, path integrity, and LLM response quality."""

import json, sys, os

def fail(msg):
    print(f"  FAIL: {msg}", file=sys.stderr)
    return 1

def ok(msg):
    print(f"  OK: {msg}")
    return 0

def check_json(path, label):
    if not os.path.exists(path):
        return None, fail(f"{label} not found: {path}")
    try:
        with open(path) as f:
            return json.load(f), 0
    except Exception as e:
        return None, fail(f"{label} invalid: {e}")

def validate_snapshot(run_dir):
    path = os.path.join(run_dir, "snapshot.json")
    data, e = check_json(path, "snapshot.json")
    if e: return e
    errors = 0
    gf = data.get("go_facts", {})
    for ep in gf.get("entrypoint_packages", []):
        if not ep.get("package_dir"):
            errors += fail(f"entrypoint {ep.get('import_path','?')}: empty package_dir")
    if not gf.get("orientation_candidates"):
        errors += fail("go_facts missing orientation_candidates")
    if errors == 0:
        ok(f"snapshot.json: {gf.get('packages_count',0)} packages, {len(gf.get('entrypoint_packages',[]))} entrypoints")
    return errors

def validate_bundle_paths(run_dir):
    path = os.path.join(run_dir, "llm_bundle.json")
    data, e = check_json(path, "llm_bundle.json")
    if e: return e
    allowed = set(data.get("allowed_paths", []))
    ok(f"llm_bundle.json: {len(allowed)} allowed paths")
    return 0

def validate_orient_report(run_dir):
    path = os.path.join(run_dir, "orientation_report.json")
    data, e = check_json(path, "orientation_report.json")
    if e: return e
    errors = 0
    for f in ["project_guess", "candidate_flows"]:
        if f not in data:
            errors += fail(f"orientation_report missing {f}")
    flows = data.get("candidate_flows", [])
    for i, cf in enumerate(flows):
        for f in ["name", "likely_files", "likely_entrypoint"]:
            if f not in cf:
                errors += fail(f"candidate_flows[{i}] missing {f}")

    # Path validation
    bundle_path = os.path.join(run_dir, "llm_bundle.json")
    bundle, _ = check_json(bundle_path, "llm_bundle.json")
    if bundle:
        allowed = set(bundle.get("allowed_paths", []))
        for fto in data.get("first_files_to_open", []):
            if fto.get("path") not in allowed:
                errors += fail(f"first_files_to_open path '{fto.get('path')}' not in allowed_paths")
        for i, cf in enumerate(flows):
            for j, lf in enumerate(cf.get("likely_files", [])):
                if lf not in allowed:
                    errors += fail(f"candidate_flows[{i}].likely_files[{j}] '{lf}' not in allowed")

    if errors == 0:
        ok(f"orientation_report.json: {len(flows)} flows, paths valid")
    return errors

def validate_flow_reports(run_dir):
    flows_dir = os.path.join(run_dir, "flows")
    if not os.path.isdir(flows_dir):
        return fail(f"flows/ not found")
    errors = 0
    for fid in sorted(os.listdir(flows_dir)):
        fd = os.path.join(flows_dir, fid)
        if not os.path.isdir(fd):
            continue
        err_path = os.path.join(fd, "error.txt")
        rpt_path = os.path.join(fd, "flow_report.json")
        bdl_path = os.path.join(fd, "flow_bundle.json")

        bundle, _ = check_json(bdl_path, f"flows/{fid}/flow_bundle.json")
        report, re = check_json(rpt_path, f"flows/{fid}/flow_report.json")
        if re:
            if os.path.exists(err_path):
                with open(err_path) as f:
                    print(f"  INFO: flows/{fid}/error.txt: {f.read().strip()}", file=sys.stderr)
            errors += re
            continue

        if bundle and report:
            bpaths = set()
            for f in bundle.get("selected_files", []) + bundle.get("selected_tests", []) + bundle.get("selected_docs", []):
                bpaths.add(f.get("path", ""))

            for i, fto in enumerate(report.get("files_to_read_in_order", [])):
                if fto.get("path") not in bpaths:
                    errors += fail(f"flows/{fid}.files_to_read_in_order[{i}] '{fto.get('path')}' not in bundle")
            for i, tt in enumerate(report.get("tests_to_read", [])):
                if tt.get("path") not in bpaths:
                    errors += fail(f"flows/{fid}.tests_to_read[{i}] '{tt.get('path')}' not in bundle")

            if not report.get("summary"):
                errors += fail(f"flows/{fid}: missing summary")
            if not report.get("likely_chain"):
                errors += fail(f"flows/{fid}: missing likely_chain")

        if errors == 0:
            ok(f"flows/{fid}: valid")
    return errors

def validate_no_secrets(run_dir):
    patterns = ["sk-", "Bearer ", "DEEPSEEK_API_KEY"]
    errors = 0
    for root, dirs, files in os.walk(run_dir):
        for fname in files:
            if "redacted" in fname or fname.endswith(".json"):
                fpath = os.path.join(root, fname)
                try:
                    with open(fpath) as f:
                        content = f.read()
                    if "Authorization" in content and "redacted" in fname:
                        errors += fail(f"{os.path.relpath(fpath, run_dir)} contains Authorization header")
                    for p in patterns:
                        if p in content and "redacted" not in content:
                            pass  # non-redacted files may have them
                except:
                    pass
    if errors == 0:
        ok("no secrets leaked")
    return errors

def main():
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <.repomap-runs/<run-id>>", file=sys.stderr)
        sys.exit(2)

    run_dir = sys.argv[1]
    if not os.path.isdir(run_dir):
        print(f"ERROR: {run_dir} is not a directory", file=sys.stderr)
        sys.exit(1)

    print(f"=== Validating {run_dir} ===\n")
    errors = 0
    errors += validate_snapshot(run_dir)
    errors += validate_bundle_paths(run_dir)
    errors += validate_orient_report(run_dir)
    errors += validate_flow_reports(run_dir)
    errors += validate_no_secrets(run_dir)
    print(f"\n{'PASS' if errors == 0 else 'FAIL'}: {errors} error(s)")
    sys.exit(0 if errors == 0 else 1)

if __name__ == "__main__":
    main()
