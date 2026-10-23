package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitexec"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/graphql"
	"github.com/4aykovski/glview/internal/gitlab/httpx"
	"github.com/4aykovski/glview/internal/gitlab/hybrid"
	"github.com/4aykovski/glview/internal/gitlab/rest"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/glab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/mrlinks"
	"github.com/4aykovski/glview/internal/startup"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/localgit"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/screens/search"
	"github.com/4aykovski/glview/internal/ui/screens/setup"
	"github.com/4aykovski/glview/internal/ui/screens/start"
	wsscreen "github.com/4aykovski/glview/internal/ui/screens/workspace"
	"github.com/4aykovski/glview/internal/workspace"
	"github.com/4aykovski/glview/internal/xdg"
)

// buildRoot resolves the start plan and returns the root model.
func buildRoot(opts options) (*app.Model, error) {
	cfgPath := opts.configPath
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfg, warns, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}

	env := app.NewEnv(i18n.Parse(cfg.Language))
	env.Cache = cache.New()
	env.Disk = &cache.Disk{Dir: xdg.CacheDir()}
	env.Catalog = app.NewCatalog(env.Disk, filepath.Join(xdg.StateDir(), "state.json"), func() string { return env.Host })
	env.Search = func(global bool) app.Overlay { return search.New(env, global) }
	var runner gitexec.Runner
	if g := gitexec.Detect(cfg.Git.Path, cfg.Git.Disabled(), cfg.Git.Timeout.D(), cfg.Git.Parallel); g != nil {
		runner = g
	}
	env.Git = localgit.New(runner)
	toasts := applyConfig(env, cfg, warns, cfgPath)

	gl := glab.Detect(cfg.Glab.Path, cfg.Glab.Disabled())
	env.Glab = gl
	var tokens config.TokenProvider
	if gl != nil {
		tokens = gl
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	plan, err := startup.Resolve(ctx, startup.Inputs{
		Config: cfg, ProjectFlag: opts.project, Cwd: cwd, Getenv: os.Getenv, Glab: tokens,
	})
	if err != nil {
		return nil, err
	}
	env.TokenSource = plan.TokenSource
	debuglog.RegisterSecret(plan.Token)
	debuglog.L().Info("plan", "mode", plan.Mode, "host", plan.HostURL, "hostSource", plan.HostSource, "tokenSource", plan.TokenSource)

	hc, err := httpx.NewClient(httpx.Options{
		CAFile:             cfg.TLS.CAFile,
		InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
		UserAgent:          "gitlab-tui/" + version,
	})
	if err != nil {
		return nil, err
	}

	connect := func(ctx context.Context, hostURL, token string) (gitlab.Client, gitlab.User, error) {
		debuglog.RegisterSecret(token)
		c, err := rest.New(hostURL, token, hc)
		if err != nil {
			return nil, gitlab.User{}, err
		}
		u, err := c.Users().Current(ctx)
		if err != nil {
			return c, u, err
		}
		gql := graphql.New(hostURL, token, hc, cfg.GraphQL.BatchSize)
		return hybrid.New(c, gql, u.Username).WithCache(env.Cache), u, nil
	}
	next := func(env *app.Env) app.Screen { return modeScreen(env, plan, cfg) }

	var first app.Screen
	var startErr error
	if plan.HostURL != "" && plan.Token != "" {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		client, user, err := connect(cctx, plan.HostURL, plan.Token)
		cancel()
		if err == nil {
			env.Client, env.User, env.Host = client, user, plan.HostURL
			first = next(env)
		} else {
			startErr = err
		}
	}
	if first == nil {
		so := setup.Options{
			Host:      plan.HostURL,
			Err:       startErr,
			Connect:   connect,
			Normalize: config.NormalizeHost,
			Next:      next,
			Save: func(hostURL, token string) error {
				values := map[string]string{"host": hostURL}
				if token != "" {
					values["token"] = token
				}
				return config.Set(cfgPath, values)
			},
		}
		if gl != nil {
			so.GlabToken = gl.Token
		}
		if errors.Is(startErr, gitlab.ErrUnauthorized) && plan.TokenSource == config.SourceGlab {
			toasts = append(toasts, app.ToastMsg{Level: common.ToastError, Text: env.Msg.Errors.Unauthorized,
				Hint: env.Msg.Errors.HintGlabToken(config.Hostname(plan.HostURL))})
		}
		first = setup.New(env, so)
	}

	return app.New(env, first, app.Options{
		SaveLanguage: func(l i18n.Lang) error {
			return config.Set(cfgPath, map[string]string{"language": string(l)})
		},
		InitialToasts: toasts,
	}), nil
}

// modeScreen builds the first screen of the resolved mode.
func modeScreen(env *app.Env, plan startup.Plan, cfg config.Config) app.Screen {
	workspaceOf := func(dir string) app.Screen {
		src := workspace.LocalDirSource{Dir: dir}
		var rows []workspace.Project
		if inst, err := gitremote.NewInstance(env.Host, cfg.HostAliases); err == nil {
			src.Instance = inst
			if rows, err = src.Projects(context.Background()); err != nil {
				debuglog.L().Info("workspace scan failed", "err", err)
			}
		}
		return wsscreen.New(env, src.Name(), workspace.NewScope(src.Name(), rows))
	}

	if plan.Mode == startup.ModeWorkspace {
		return workspaceOf(plan.WorkspaceDir)
	}

	target, err := plan.Target(env.Host, cfg.HostAliases)
	if err != nil || target.ProjectPath == "" {
		info := start.NotOnInstanceInfo{Reason: env.Msg.Startup.NotOnInstance(config.Hostname(env.Host))}
		if plan.Repo != nil {
			info.RemoteURL = plan.Repo.RemoteURL
		}
		if plan.Remote != nil {
			info.RemoteHost = plan.Remote.Host
		}
		if target.Reason == startup.TargetNoRemote {
			info.Reason = env.Msg.Startup.NoUsableRemote
		}
		parent := plan.ParentDir()
		return start.NewNotOnInstance(env, info, func() app.Screen { return workspaceOf(parent) })
	}
	env.Catalog.SetCurrent(target.ProjectPath, plan.Repo)
	return project.New(env, project.Options{Path: target.ProjectPath, Repo: plan.Repo})
}

// applyConfig copies the config values screens need into env.Settings and
// turns the load warnings into toasts.
func applyConfig(env *app.Env, cfg config.Config, warns []config.Warning, cfgPath string) []app.ToastMsg {
	env.Settings = app.Settings{
		PageSize: cfg.Project.PageSize, PollActive: cfg.Polling.Active.D(), PollIdle: cfg.Polling.Idle.D(),
		PollWorkspace: cfg.Polling.Workspace.D(), PerProject: cfg.Workspace.PerProject, MaxGroupProjects: cfg.Workspace.MaxGroupProjects,
		FetchOnStart:       cfg.Git.FetchOnStart,
		RefsTagsFirst:      cfg.RunPipeline.RefsFirst == config.RefsTagsFirst,
		PrefillLocalBranch: cfg.RunPipeline.PrefillLocalBranch,
		LinkRules:          linkRules(cfg.Links),
	}
	var toasts []app.ToastMsg
	for _, w := range warns {
		var text string
		switch w.Kind {
		case config.WarnPermissions:
			text = env.Msg.Toast.ConfigPermissions(cfgPath)
		case config.WarnLink:
			text = env.Msg.Links.BadRule(w.Text)
		case config.WarnValue:
			text = env.Msg.Toast.ConfigBadValue(w.Key, w.Value, w.Default)
		default:
			text = env.Msg.Toast.ConfigBroken(w.Text)
		}
		toasts = append(toasts, app.ToastMsg{Level: common.ToastWarn, Text: text})
	}
	return toasts
}

// linkRules compiles the links rules of the config; Load has already
// dropped the invalid ones.
func linkRules(rules []config.LinkRule) []mrlinks.Rule {
	var out []mrlinks.Rule
	for _, r := range rules {
		if re, err := regexp.Compile(r.Pattern); err == nil {
			out = append(out, mrlinks.Rule{Re: re, URL: r.URL})
		}
	}
	return out
}
