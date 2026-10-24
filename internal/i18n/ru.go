package i18n

import "fmt"

var ru = func() Messages {
	var m Messages
	m.Lang = RU
	m.Name = "Русский"

	m.Tabs.Projects = "Проекты"
	m.Tabs.MRs = "MRs"
	m.Tabs.Pipelines = "Pipelines"
	m.Tabs.Issues = "Issues"
	m.Tabs.Tags = "Tags"

	m.Keys.Up = "вверх"
	m.Keys.Down = "вниз"
	m.Keys.Top = "в начало"
	m.Keys.Bottom = "в конец"
	m.Keys.Open = "открыть"
	m.Keys.Back = "назад"
	m.Keys.Quit = "выход"
	m.Keys.Refresh = "обновить"
	m.Keys.Browser = "браузер"
	m.Keys.Copy = "копировать"
	m.Keys.Help = "хелп"
	m.Keys.Language = "lang: EN"
	m.Keys.NextTab = "след. таб"
	m.Keys.PrevTab = "пред. таб"
	m.Keys.Tabs = "табы"
	m.Keys.Project = "проект"
	m.Keys.Search = "поиск по GitLab"
	m.Keys.Me = "я"

	m.Help.Title = "Хоткеи"
	m.Help.Screen = "Этот экран"
	m.Help.Global = "Везде"
	m.Help.Close = "esc / ? закрыть"
	m.Help.Scroll = "j/k прокрутка"
	m.Help.LanguageLine = "Язык: русский · ctrl+l — английский"
	m.Help.TokenSource = "Токен из"
	m.Help.GlabFound = func(host string) string { return "glab: найден, используется для " + host }
	m.Help.GlabNotFound = "glab не найден — checkout недоступен"

	m.Confirm.Yes = "Да"
	m.Confirm.No = "Отмена"

	m.Header.Workspace = "workspace"
	m.Header.Updated = func(age string) string { return "⟳ " + age }
	m.Header.Offline = "офлайн"
	m.Header.AuthError = "✗ авторизация"

	m.Toast.LanguageSwitched = "Язык: русский"
	m.Toast.ConfigSaveFailed = func(err string) string { return "Не удалось сохранить конфиг: " + err }
	m.Toast.ConfigBroken = func(detail string) string {
		return "Конфиг — не валидный YAML, использую значения по умолчанию: " + detail
	}
	m.Toast.ConfigPermissions = func(path string) string {
		return path + " содержит токен и доступен другим: chmod 600"
	}
	m.Toast.ConfigBadValue = func(key, value, def string) string {
		return fmt.Sprintf("Конфиг: неизвестное значение %q у %s, использую %s", value, key, def)
	}
	m.Toast.BrowserFailed = "Не удалось открыть браузер"
	m.Toast.Copied = func(what string) string { return "скопировано: " + what }
	m.Toast.AuthRestored = "Токен принят"
	m.Toast.Online = "Соединение восстановлено"

	m.Errors.Unauthorized = "Токен не принят"
	m.Errors.Forbidden = "Недостаточно прав"
	m.Errors.InsufficientScope = "У токена нет scope api (только чтение)"
	m.Errors.NotFound = "Не найдено или нет доступа"
	m.Errors.Conflict = "Конфликт: объект изменился, обнови (r)"
	m.Errors.NotPossible = "Операция невозможна"
	m.Errors.Network = "Хост недоступен"
	m.Errors.TLS = "Ошибка TLS"
	m.Errors.Server = "Ошибка сервера GitLab"
	m.Errors.Unknown = "Неожиданная ошибка"
	m.Errors.RateLimited = func(s int) string {
		return fmt.Sprintf("GitLab ограничил запросы, повтор через %d с", s)
	}
	m.Errors.HintConfigToken = "обнови token в config.yaml"
	m.Errors.HintEnvToken = "проверь GITLAB_TOKEN"
	m.Errors.HintGlabToken = func(host string) string { return "выполни: glab auth login --hostname " + host }
	m.Errors.HintTLS = "укажи tls.ca_file в config.yaml"

	m.Setup.Title = "Подключение к GitLab"
	m.Setup.Intro = "Токен не найден ни в конфиге, ни в GITLAB_TOKEN, ни в glab."
	m.Setup.URL = "URL инстанса"
	m.Setup.URLHint = "например, https://gitlab.example.com"
	m.Setup.Token = "Personal access token"
	m.Setup.TokenHint = "нужен scope api; read_api — только чтение"
	m.Setup.Submit = "Подключиться"
	m.Setup.Checking = "Проверяю токен…"
	m.Setup.BadToken = "Токен неверный или истёк"
	m.Setup.Unreachable = "Хост недоступен"
	m.Setup.TLSFailed = "Ошибка TLS: укажи tls.ca_file в config.yaml"
	m.Setup.Saved = "Сохранено в конфиг"
	m.Setup.UseGlab = func(host string) string { return "Взять токен из glab для " + host }
	m.Setup.LoggedInAs = func(user string) string { return "Вошли как @" + user }

	m.Startup.NotOnInstance = func(instance string) string { return "Проект не на " + instance }
	m.Startup.Remote = "remote"
	m.Startup.RemoteHost = "хост remote"
	m.Startup.InstanceHost = "настроенный инстанс"
	m.Startup.ProjectNotFound = "Проект не найден или нет доступа"
	m.Startup.NoUsableRemote = "Нет подходящего remote (нет origin, и remote не один)"
	m.Startup.EmptyWorkspace = func(dir string) string { return "В " + dir + " нет GitLab-проектов" }
	m.Startup.EmptyWorkspaceDetail = func(scanned, foreign, noGit int) string {
		return fmt.Sprintf("просмотрено %s, из них %d с чужим remote, %d без .git",
			Count(RU, scanned, PluralForms{"папка", "папки", "папок"}), foreign, noGit)
	}
	m.Startup.TooSmall = func(w, h int) string { return fmt.Sprintf("Расширь терминал до %d×%d", w, h) }
	m.Startup.PickProject = "выбрать проект"
	m.Startup.GlobalSearch = "поиск по GitLab"
	m.Startup.OpenWorkspace = "workspace родительской папки"
	m.Startup.SkippedNoGit = func(names string) string { return "пропущено без .git: " + names }
	m.Startup.ForeignRemote = "не GitLab"
	m.Startup.NoRemote = "нет remote"

	m.Columns.Folder = "Папка"
	m.Columns.GitLab = "GitLab"
	m.Columns.LocalBranch = "Локальная ветка"

	m.Fields.Project = "проект"
	m.Fields.Web = "web"
	m.Fields.DefaultBranch = "default-ветка"
	m.Fields.LocalDir = "локальная папка"
	m.Fields.Branch = "ветка"
	m.Fields.Activity = "активность"
	m.Fields.Detached = "detached"

	m.Plural.GitLabProjects = func(n int) string {
		return Count(RU, n, PluralForms{"GitLab-проект", "GitLab-проекта", "GitLab-проектов"})
	}
	m.Plural.Projects = func(n int) string {
		return Count(RU, n, PluralForms{"проект", "проекта", "проектов"})
	}
	m.Plural.More = func(n int) string { return fmt.Sprintf("ещё %d", n) }
	m.Plural.Tags = func(n int) string { return Count(RU, n, PluralForms{"тег", "тега", "тегов"}) }
	m.Plural.Lines = func(n int) string { return Count(RU, n, PluralForms{"строка", "строки", "строк"}) }
	fillM1RU(&m)
	fillM2RU(&m)
	fillM3RU(&m)
	fillT37RU(&m)
	return m
}()
