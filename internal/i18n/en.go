package i18n

import "fmt"

var en = func() Messages {
	var m Messages
	m.Lang = EN
	m.Name = "English"

	m.Tabs.Projects = "Projects"
	m.Tabs.MRs = "MRs"
	m.Tabs.Pipelines = "Pipelines"
	m.Tabs.Issues = "Issues"
	m.Tabs.Tags = "Tags"

	m.Keys.Up = "up"
	m.Keys.Down = "down"
	m.Keys.Top = "top"
	m.Keys.Bottom = "bottom"
	m.Keys.Open = "open"
	m.Keys.Back = "back"
	m.Keys.Quit = "quit"
	m.Keys.Refresh = "refresh"
	m.Keys.Browser = "browser"
	m.Keys.Copy = "copy"
	m.Keys.Help = "help"
	m.Keys.Language = "язык: RU"
	m.Keys.NextTab = "next tab"
	m.Keys.PrevTab = "prev tab"
	m.Keys.Tabs = "tabs"
	m.Keys.Project = "project"
	m.Keys.Search = "search GitLab"
	m.Keys.Me = "me"

	m.Help.Title = "Keys"
	m.Help.Screen = "This screen"
	m.Help.Global = "Everywhere"
	m.Help.Close = "esc / ? close"
	m.Help.LanguageLine = "Language: English · ctrl+l switches to Russian"
	m.Help.TokenSource = "Token from"
	m.Help.GlabFound = func(host string) string { return "glab: found, used for " + host }
	m.Help.GlabNotFound = "glab: not found — checkout is unavailable"

	m.Confirm.Yes = "Yes"
	m.Confirm.No = "Cancel"

	m.Header.Workspace = "workspace"
	m.Header.Updated = func(age string) string { return "⟳ " + age }
	m.Header.Offline = "offline"
	m.Header.AuthError = "✗ auth"

	m.Toast.LanguageSwitched = "Language: English"
	m.Toast.ConfigSaveFailed = func(err string) string { return "Could not save config: " + err }
	m.Toast.ConfigBroken = func(detail string) string { return "Config is not valid YAML, using defaults: " + detail }
	m.Toast.ConfigPermissions = func(path string) string { return path + " holds a token and is readable by others: chmod 600" }
	m.Toast.ConfigBadValue = func(key, value, def string) string {
		return fmt.Sprintf("Config: unknown value %q of %s, using %s", value, key, def)
	}
	m.Toast.BrowserFailed = "Could not open the browser"
	m.Toast.Copied = func(what string) string { return "copied: " + what }
	m.Toast.AuthRestored = "Token accepted"

	m.Errors.Unauthorized = "Token rejected"
	m.Errors.Forbidden = "Not enough permissions"
	m.Errors.InsufficientScope = "The token lacks the api scope (read-only)"
	m.Errors.NotFound = "Not found or no access"
	m.Errors.Conflict = "Conflict: the object changed, refresh (r)"
	m.Errors.NotPossible = "Operation not possible"
	m.Errors.Network = "Host unreachable"
	m.Errors.TLS = "TLS error"
	m.Errors.Server = "GitLab server error"
	m.Errors.Unknown = "Unexpected error"
	m.Errors.RateLimited = func(s int) string { return fmt.Sprintf("GitLab rate limit, retry in %ds", s) }
	m.Errors.HintConfigToken = "update token in config.yaml"
	m.Errors.HintEnvToken = "check GITLAB_TOKEN"
	m.Errors.HintGlabToken = func(host string) string { return "run: glab auth login --hostname " + host }
	m.Errors.HintTLS = "set tls.ca_file in config.yaml"

	m.Setup.Title = "Connect to GitLab"
	m.Setup.Intro = "No token found in config, GITLAB_TOKEN or glab."
	m.Setup.URL = "Instance URL"
	m.Setup.URLHint = "e.g. https://gitlab.example.com"
	m.Setup.Token = "Personal access token"
	m.Setup.TokenHint = "scope api; read_api is read-only"
	m.Setup.Submit = "Connect"
	m.Setup.Checking = "Checking token…"
	m.Setup.BadToken = "Token is invalid or expired"
	m.Setup.Unreachable = "Host unreachable"
	m.Setup.TLSFailed = "TLS error: set tls.ca_file in config.yaml"
	m.Setup.Saved = "Saved to config"
	m.Setup.UseGlab = func(host string) string { return "Use token from glab for " + host }
	m.Setup.LoggedInAs = func(user string) string { return "Logged in as @" + user }

	m.Startup.NotOnInstance = func(instance string) string { return "This project is not on " + instance }
	m.Startup.Remote = "remote"
	m.Startup.RemoteHost = "remote host"
	m.Startup.InstanceHost = "configured instance"
	m.Startup.ProjectNotFound = "Project not found or no access"
	m.Startup.NoUsableRemote = "No usable remote (no origin and not exactly one remote)"
	m.Startup.EmptyWorkspace = func(dir string) string { return "No GitLab projects in " + dir }
	m.Startup.EmptyWorkspaceDetail = func(scanned, foreign, noGit int) string {
		return fmt.Sprintf("scanned %s: %d with another remote, %d without .git",
			Count(EN, scanned, PluralForms{"folder", "", "folders"}), foreign, noGit)
	}
	m.Startup.TooSmall = func(w, h int) string { return fmt.Sprintf("Make the terminal at least %d×%d", w, h) }
	m.Startup.PickProject = "pick project"
	m.Startup.GlobalSearch = "search GitLab"
	m.Startup.OpenWorkspace = "workspace of parent folder"
	m.Startup.ComingSoon = "coming in the next milestone"
	m.Startup.SkippedNoGit = func(names string) string { return "skipped without .git: " + names }
	m.Startup.ForeignRemote = "not GitLab"
	m.Startup.NoRemote = "no remote"

	m.Columns.Folder = "Folder"
	m.Columns.GitLab = "GitLab"
	m.Columns.LocalBranch = "Local branch"

	m.Fields.Project = "project"
	m.Fields.Web = "web"
	m.Fields.DefaultBranch = "default branch"
	m.Fields.LocalDir = "local folder"
	m.Fields.Branch = "branch"
	m.Fields.Activity = "activity"
	m.Fields.Detached = "detached"

	m.Plural.GitLabProjects = func(n int) string {
		return Count(EN, n, PluralForms{"GitLab project", "", "GitLab projects"})
	}
	m.Plural.Projects = func(n int) string { return Count(EN, n, PluralForms{"project", "", "projects"}) }
	m.Plural.More = func(n int) string { return fmt.Sprintf("%d more", n) }
	m.Plural.Tags = func(n int) string { return Count(EN, n, PluralForms{"tag", "", "tags"}) }
	m.Plural.Lines = func(n int) string { return Count(EN, n, PluralForms{"line", "", "lines"}) }
	fillM1EN(&m)
	return m
}()
