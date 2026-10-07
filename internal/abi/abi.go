package abi

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"skein/internal/canonical"
	"skein/internal/repo"
	"skein/internal/skeinerr"
)

func Run(args []string, stdin io.Reader) (map[string]any, int) {
	out, err := run(args, stdin)
	if err != nil {
		code := "malformed"
		msg := err.Error()
		if e, ok := skeinerr.As(err); ok {
			code = e.Code
			msg = e.Message
			if msg == "" {
				msg = e.Code
			}
		}
		return map[string]any{"ok": false, "error": code, "message": msg}, 1
	}
	if out == nil {
		out = map[string]any{"ok": true}
	}
	if _, ok := out["ok"]; !ok {
		out["ok"] = true
	}
	return out, 0
}

func Write(w io.Writer, obj map[string]any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(obj)
}

func run(args []string, stdin io.Reader) (map[string]any, error) {
	dir := ""
	rest := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--dir" && i+1 < len(args) {
			i++
			dir = args[i]
			continue
		}
		if len(a) > 6 && a[:6] == "--dir=" {
			dir = a[6:]
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) == 0 {
		return nil, skeinerr.New(skeinerr.Malformed, "command required")
	}
	cmd := rest[0]
	flags := parseFlags(rest[1:])
	if dir == "" {
		dir = flags["dir"]
	}
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, skeinerr.New(skeinerr.IO, err.Error())
		}
		dir = wd
	}
	if cmd == "init" {
		mode := flags["path-mode"]
		r, out, err := repo.Init(dir, mode)
		if err == nil {
			repo.AnnotateHuman(out)
			_ = r
		}
		return out, err
	}
	root, err := repo.FindRoot(dir)
	if err != nil {
		return nil, err
	}
	r, err := repo.Open(root)
	if err != nil {
		return nil, err
	}
	out, err := dispatch(r, cmd, flags, stdin)
	if err == nil {
		r.Annotate(out)
	}
	return out, err
}

func dispatch(r *repo.Repo, cmd string, flags map[string]string, stdin io.Reader) (map[string]any, error) {
	switch cmd {
	case "open-thread":
		return r.OpenThread(flags["title"], orDefault(flags["scale"], "trivial"), orDefault(flags["kind"], "feature"))
	case "threads":
		return r.ListThreads()
	case "checkout":
		thread, err := optionalThread(r, flags["thread"])
		if err != nil {
			return nil, err
		}
		weave := flags["weave"]
		if weave != "" {
			weave, err = r.ResolveKind("weave", weave)
			if err != nil {
				return nil, err
			}
		}
		return r.Checkout(weave, orDefault(flags["ref"], "main"), thread, flags["out"])
	case "sync":
		return r.Sync(flags["checkout"])
	case "assert":
		thread, err := optionalThread(r, flags["thread"])
		if err != nil {
			return nil, err
		}
		return r.Assert(flags["type"], thread, flags["body-file"])
	case "test":
		thread, err := optionalThread(r, flags["thread"])
		if err != nil {
			return nil, err
		}
		return r.Test(thread, flags["strategy"], flags["patch"])
	case "propose-land":
		thread, err := optionalThread(r, flags["thread"])
		if err != nil {
			return nil, err
		}
		return r.ProposeLand(thread)
	case "approve":
		wp, err := r.ResolveKind("weave-proposal", flags["weave-proposal"])
		if err != nil {
			return nil, err
		}
		return r.Approve(wp)
	case "land":
		wp, err := r.ResolveKind("weave-proposal", flags["weave-proposal"])
		if err != nil {
			return nil, err
		}
		return r.Land(wp)
	case "log":
		limit := 0
		if flags["limit"] != "" {
			for _, c := range flags["limit"] {
				if c >= '0' && c <= '9' {
					limit = limit*10 + int(c-'0')
				}
			}
		}
		return r.Log(orDefault(flags["ref"], "main"), limit)
	case "diff":
		return r.Diff(flags["from"], flags["to"])
	case "why":
		return r.Why(flags["path"], orDefault(flags["ref"], "main"))
	case "agent":
		return runAgent(r, stdin)
	default:
		return nil, skeinerr.New(skeinerr.Malformed, "unknown command: "+cmd)
	}
}

func optionalThread(r *repo.Repo, spec string) (string, error) {
	if spec == "" {
		return "", nil
	}
	return r.ResolveThread(spec)
}

func runAgent(r *repo.Repo, stdin io.Reader) (map[string]any, error) {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	v, err := canonical.DecodeJSON(raw)
	if err != nil {
		return nil, err
	}
	m, ok := canonical.AsMap(v)
	if !ok {
		return nil, skeinerr.New(skeinerr.Malformed, "agent payload must be an object")
	}
	verb, _ := canonical.AsString(m["verb"])
	if verb == "" {
		verb, _ = canonical.AsString(m["op"])
	}
	switch verb {
	case "open-thread":
		title, _ := canonical.AsString(m["title"])
		scale, _ := canonical.AsString(m["scale"])
		kind, _ := canonical.AsString(m["kind"])
		return r.OpenThread(title, orDefault(scale, "trivial"), orDefault(kind, "feature"))
	case "assert":
		typ, _ := canonical.AsString(m["type"])
		thread, _ := canonical.AsString(m["thread"])
		thread, err = optionalThread(r, thread)
		if err != nil {
			return nil, err
		}
		tmp, err := os.CreateTemp("", "skein-body-*.json")
		if err != nil {
			return nil, skeinerr.New(skeinerr.IO, err.Error())
		}
		body := m["body"]
		b, err := json.Marshal(body)
		if err != nil {
			return nil, skeinerr.New(skeinerr.Malformed, err.Error())
		}
		_ = os.WriteFile(tmp.Name(), b, 0o600)
		defer os.Remove(tmp.Name())
		return r.Assert(typ, thread, tmp.Name())
	case "project":
		thread, _ := canonical.AsString(m["thread"])
		thread, err = optionalThread(r, thread)
		if err != nil {
			return nil, err
		}
		reach, _ := canonical.AsString(m["reach"])
		if reach == "" {
			reach = "checkout-readable"
		}
		weaveID, err := r.MainWeaveID()
		if err != nil {
			return nil, err
		}
		w, err := r.LoadWeave(weaveID)
		if err != nil {
			return nil, err
		}
		ctx := map[string]any{
			"schema_version":     int64(1),
			"kind":               "context",
			"source_weave_id":    weaveID,
			"source_state_root":  w["state_root"],
			"selected_claim_ids": []any{thread},
			"reach":              reach,
			"enforcement":        map[string]any{"mechanism": "unspecified-local"},
		}
		id, err := r.PutWitness(ctx, false)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "context_witness_id": id, "bundle": map[string]any{"thread": thread, "reach": reach}}, nil
	case "witness":
		thread, _ := canonical.AsString(m["thread"])
		thread, err = optionalThread(r, thread)
		if err != nil {
			return nil, err
		}
		return r.Test(thread, "generic", "")
	case "sync":
		co, _ := canonical.AsString(m["checkout"])
		return r.Sync(co)
	case "propose-land":
		thread, _ := canonical.AsString(m["thread"])
		thread, err = optionalThread(r, thread)
		if err != nil {
			return nil, err
		}
		return r.ProposeLand(thread)
	case "gate":
		wp, _ := canonical.AsString(m["weave_proposal_id"])
		wp, err = r.ResolveKind("weave-proposal", wp)
		if err != nil {
			return nil, err
		}
		return r.Approve(wp)
	case "land":
		wp, _ := canonical.AsString(m["weave_proposal_id"])
		wp, err = r.ResolveKind("weave-proposal", wp)
		if err != nil {
			return nil, err
		}
		return r.Land(wp)
	case "query":
		if p, ok := canonical.AsString(m["why_path"]); ok && p != "" {
			return r.Why(p, "main")
		}
		return r.Log("main", 0)
	case "checkout":
		weave, _ := canonical.AsString(m["weave"])
		thread, _ := canonical.AsString(m["thread"])
		thread, err = optionalThread(r, thread)
		if err != nil {
			return nil, err
		}
		return r.Checkout(weave, "main", thread, "")
	case "port":
		return nil, skeinerr.New(skeinerr.Policy, "port is not in Phase 1")
	default:
		return nil, skeinerr.New(skeinerr.Malformed, "unknown agent verb: "+verb)
	}
}

func parseFlags(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) < 3 || a[:2] != "--" {
			continue
		}
		body := a[2:]
		if eq := indexByte(body, '='); eq >= 0 {
			out[body[:eq]] = body[eq+1:]
			continue
		}
		if i+1 < len(args) && (len(args[i+1]) < 2 || args[i+1][:2] != "--") {
			out[body] = args[i+1]
			i++
			continue
		}
		out[body] = "true"
	}
	return out
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func Abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}
