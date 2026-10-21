package i18n

import "fmt"

func fillM2RU(m *Messages) {
	w := &m.WS
	w.ShownOf = func(shown, total int) string { return fmt.Sprintf("показаны %d из %d", shown, total) }
	w.ShownOfMany = func(shown int) string { return fmt.Sprintf("показаны %d из многих", shown) }
	w.ColFolder, w.ColGitLab, w.ColBranch, w.ColAheadBehind = "Папка", "GitLab", "Локальная ветка", "↑↓"
	w.ColMRs, w.ColDefault, w.ColTag, w.ColActivity = "MRs", "default-ветка", "Тег", "Акт"
	w.NoAccess = "нет доступа"
	w.Archived = "archived"
	w.EmptyHint = "p выбрать проект · F поиск по GitLab · q выход"
	w.FilterHint = "текст в имени папки или пути проекта · enter применить · esc отмена"
	w.Search = "поиск"
	w.PrevRemote = "remote"
	w.PrevOpenMRs = func(open, review int) string {
		s := fmt.Sprintf("%d open MR", open)
		if review > 0 {
			s += fmt.Sprintf(" (%d ждут твоего ревью)", review)
		}
		return s
	}
	w.PrevLocal = func(branch string) string { return "локально на " + branch }
	w.PrevNoMR = "нет открытого MR"
	w.PrevLoading = "◐ загрузка…"
	w.PrevNoPipe = "нет пайплайнов"
	w.GroupCount = func(n int, state string, atLeast bool) string {
		if state == "opened" {
			state = "open"
		}
		if atLeast {
			return fmt.Sprintf("≥%d %s", n, state)
		}
		return fmt.Sprintf("%d %s", n, state)
	}
	w.GroupWaiting = func(n int) string { return fmt.Sprintf("%d ждут тебя", n) }
	w.GroupShown = func(n int) string { return fmt.Sprintf("показано %d", n) }
	w.More = func(n int) string { return fmt.Sprintf("↳ ещё %d · f фильтр по проекту", n) }
	w.EmptyMRs = func(state string, mine bool) string {
		words := map[string]string{"opened": "открытых", "merged": "смерженных", "closed": "закрытых", "all": ""}
		word, ok := words[state]
		if !ok {
			word = state
		}
		switch {
		case mine && word != "":
			return "нет твоих " + word + " MR"
		case mine:
			return "нет твоих MR"
		case word == "":
			return "нет MR"
		}
		return "нет " + word
	}
	w.EmptyPipes = func(mine bool) string {
		if mine {
			return "нет твоих пайплайнов"
		}
		return "нет пайплайнов"
	}
	w.Loading = "загрузка…"
	w.ColYou = "Ты"
	w.MeMRs = "я: author/assignee/reviewer"
	w.MePipes = "я: мои"
	w.StatusGroups = "группы по последней активности"
	w.ProjectKey = "проект"
	w.ProjectHint = "fuzzy по пути проекта · enter применить · esc закрыть"
	w.ShownProjects = func(shown, total int) string {
		return fmt.Sprintf("показаны %d из %d %s", shown, total, Plural(RU, total, PluralForms{"проекта", "проектов", "проектов"}))
	}
	w.FoundIn = func(found, total int) string {
		return fmt.Sprintf("найдено в %d %s из %d", found, Plural(RU, found, PluralForms{"проекте", "проектах", "проектах"}), total)
	}
	w.NothingFound = "ничего не найдено"
	w.KeyGroups, w.KeyCollapse, w.KeyProject, w.KeyOpenProject = "между проектами", "развернуть/свернуть", "фильтр по проекту", "открыть проект"
	w.KeyProjects = "фильтр проектов"

	s := &m.Search
	s.PickerTitle, s.GlobalTitle = "Сменить проект", "Поиск по GitLab"
	s.SecRemote, s.SecWorkspace, s.SecStarred, s.SecRecent = "ИЗ GIT REMOTE", "WORKSPACE", "STARRED", "НЕДАВНИЕ"
	s.SecProjects, s.SecGroups = "Проекты", "Группы"
	s.Current, s.Local = "текущий", "локально"
	s.MRs = func(n int) string { return fmt.Sprintf("%d MR", n) }
	s.NothingFound = "ничего не найдено — F поиск по GitLab"
	s.NothingGlobal = "ничего не найдено"
	s.TypeMore = "введи хотя бы 3 символа"
	s.Searching = "◐ ищем…"
	s.PickerHints = "↑↓ выбор  enter открыть  esc закрыть"
	s.GlobalHints = "↑↓ выбор  enter открыть (группа — как workspace)  esc закрыть"
	s.Failed = func(err string) string { return "✗ поиск не удался: " + err }
	s.Counter = func(shown, total int) string { return fmt.Sprintf("%d / %d", shown, total) }
	s.KeyPicker, s.KeyGlobal = "выбрать проект", "поиск по GitLab"

	g := &m.Git
	g.Progress = func(op string, done, total int) string { return fmt.Sprintf("⟳ %s %d/%d", op, done, total) }
	g.Busy = func(op string) string { return op + " уже идёт" }
	g.Summary = func(op string, ok, skipped, failed int, first string) string {
		s := fmt.Sprintf("%s: %d ✓", op, ok)
		if skipped > 0 {
			s += fmt.Sprintf(", %d ○", skipped)
		}
		if failed > 0 {
			s += fmt.Sprintf(", %d ✗", failed)
		}
		if first != "" {
			s += " (" + first + ")"
		}
		return s
	}
	g.ReasonAuth, g.ReasonNetwork, g.ReasonDiverged = "нужна авторизация", "сеть", "разошлась"
	g.ReasonDirty, g.ReasonTimeout = "мешают локальные изменения", "таймаут"
	g.ReasonDetached, g.ReasonNoUpstream, g.ReasonInProgress, g.ReasonNoRemote = "detached", "нет upstream", "идёт merge или rebase", "нет remote"
	g.AuthHint = func(dir string) string {
		return "выполни git fetch в " + dir + " вручную (ssh-ключ, known_hosts)"
	}
	g.UpToDate = "актуально"
	g.Moved = func(n int) string { return fmt.Sprintf("+%d", n) }
	g.PullTitle = "Pull (только fast-forward)?"
	g.PullBody = func(update, skip int, reasons string) string {
		s := "обновить " + Count(RU, update, PluralForms{"репозиторий", "репозитория", "репозиториев"})
		if skip > 0 {
			s += fmt.Sprintf(", пропустить %d (%s)", skip, reasons)
		}
		return s
	}
	g.KeyFetch, g.KeyPull = "fetch", "pull ff-only"
}
