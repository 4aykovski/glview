package main

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/graphql"
	"github.com/4aykovski/glview/internal/gitlab/httpx"
	"github.com/4aykovski/glview/internal/gitlab/hybrid"
	"github.com/4aykovski/glview/internal/gitlab/rest"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/glab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/startup"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/screens/setup"
	"github.com/4aykovski/glview/internal/ui/screens/start"
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
	env.Settings = app.Settings{PageSize: cfg.Project.PageSize, PollActive: cfg.Polling.Active.D(), PollIdle: cfg.Polling.Idle.D()}
	var toasts []app.ToastMsg
	for _, w := range warns {
		text := env.Msg.Toast.ConfigBroken(w.Text)
		if w.Kind == config.WarnPermissions {
			text = env.Msg.Toast.ConfigPermissions(cfgPath)
		}
		toasts = append(toasts, app.ToastMsg{Level: common.ToastWarn, Text: text})
	}

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
		return hybrid.New(c, gql, u.Username), u, nil
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
		inst, err := gitremote.NewInstance(env.Host, cfg.HostAliases)
		if err != nil {
			return start.NewEmptyWorkspace(env, dir, 0, 0, 0)
		}
		entries, err := gitremote.Scan(dir, inst)
		if err != nil {
			return start.NewEmptyWorkspace(env, dir, 0, 0, 0)
		}
		ws := start.NewWorkspace(env, dir, entries)
		if ws.GitLabCount() == 0 {
			var foreign, noGit int
			for _, e := range entries {
				if e.Kind == gitremote.KindNoGit {
					noGit++
				} else {
					foreign++
				}
			}
			return start.NewEmptyWorkspace(env, dir, len(entries), foreign, noGit)
		}
		return ws
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
	return project.New(env, target.ProjectPath, plan.Repo)
}
