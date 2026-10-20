package i18n

import "fmt"

func fillM1RU(m *Messages) {
	k := &m.Keys
	k.Filter = "фильтр"
	k.Approve = "апрув"
	k.Merge = "merge"
	k.Rebase = "rebase"
	k.Checkout = "checkout"
	k.Retry = "повтор"
	k.Cancel = "отмена"
	k.NewPipeline = "новый пайплайн"
	k.Log = "лог"
	k.Play = "запуск"
	k.Follow = "следить"
	k.Threads = "треды"
	k.Fold = "свернуть"
	k.FoldAll = "свернуть все"
	k.Stage = "стейдж"
	k.Job = "джоба"
	k.Edit = "правка"
	k.Add = "добавить"
	k.Delete = "удалить"
	k.Submit = "запустить"
	k.Next = "далее"
	k.Details = "детали"
	k.Attempts = "попытки"
	k.State, k.Status, k.Reset = "состояние", "статус", "сброс фильтров"
	k.BrowserList, k.BrowserOne, k.BrowserJob = "список в браузере", "пайплайн в браузере", "джоба в браузере"
	k.Find = "поиск по логу"
	k.Comment = "коммент"

	m.Plural.MRs = func(n int) string { return Count(RU, n, PluralForms{"MR", "MR", "MR"}) }
	m.Plural.Pipelines = func(n int) string {
		return Count(RU, n, PluralForms{"пайплайн", "пайплайна", "пайплайнов"})
	}
	m.Plural.Jobs = func(n int) string { return Count(RU, n, PluralForms{"джоба", "джобы", "джоб"}) }
	m.Plural.Threads = func(n int) string { return Count(RU, n, PluralForms{"тред", "треда", "тредов"}) }
	m.Plural.Replies = func(n int) string { return Count(RU, n, PluralForms{"ответ", "ответа", "ответов"}) }
	m.Plural.Files = func(n int) string { return Count(RU, n, PluralForms{"файл", "файла", "файлов"}) }

	p := &m.Project
	p.Later = func(tab string) string { return tab + " появятся здесь позже" }
	p.LoadingMore = "◐ загружаю ещё…"
	p.LoadFailed = "✗ не удалось загрузить, r — повторить"
	p.Loading = "◐ загружаю…"
	p.Counter = func(loaded, total int, atLeast bool) string {
		if total < 0 {
			return fmt.Sprintf("загружено %d", loaded)
		}
		t := groupThousands(total, ' ')
		if atLeast {
			t = "≥" + t
		}
		return fmt.Sprintf("%d из %s", loaded, t)
	}
	p.FilterHint = "ключ:значение … текст · tab дополняет · enter применяет · esc закрывает"
	p.FilterUnknown = func(key, known string) string {
		return fmt.Sprintf("неизвестный ключ %q, известные: %s", key, known)
	}
	p.FilterBad = func(key, value, allowed string) string {
		if allowed == "" {
			return fmt.Sprintf("%s: значение %q не подходит", key, value)
		}
		return fmt.Sprintf("%s: %q не подходит, допустимо: %s", key, value, allowed)
	}
	p.PreviewHidden = "расширь терминал, чтобы увидеть превью"
	p.KeyState, p.KeyScope, p.KeyReviewer, p.KeyDraft, p.KeyLabels = "состояние MR", "чьи MR", "ревьюер", "черновик или нет", "метки"
	p.KeyRef, p.KeyStatus, p.KeySource, p.KeyUser = "ветка или тег", "статус пайплайна", "что запустило", "кто запустил"
	p.StatusPoll = func(i string) string { return "опрос " + i }
	p.StatusLive = func(n int, i string) string { return fmt.Sprintf("live: %d running · опрос %s", n, i) }
	p.NoteBranch, p.NoteTag = "ветка", "тег"

	pp := &m.Pipe
	pp.EmptyBase, pp.EmptyWhere = "Нет пайплайнов", ", где "
	pp.QualRef = func(v string) string { return "ref " + v }
	pp.QualStatus = func(v string) string { return "статус " + v }
	pp.QualSource = func(v string) string { return "источник " + v }
	pp.QualUser = func(v string) string { return "запустил @" + v }
	pp.QualMine = "запустил ты"
	pp.EmptyHint = "/ меняет фильтры · m — только мои"
	pp.RetryTitle = func(id int64) string {
		return fmt.Sprintf("Перезапустить упавшие джобы пайплайна #%d?", id)
	}
	pp.CancelTitle = func(id int64) string { return fmt.Sprintf("Отменить пайплайн #%d?", id) }
	pp.RetryBody = "Упавшие джобы пайплайна будут запущены заново."
	pp.CancelBody = "Выполняющиеся и ожидающие джобы будут отменены."
	pp.ToastRetried = func(id int64) string { return fmt.Sprintf("перезапускаю пайплайн #%d", id) }
	pp.ToastCanceled = func(id int64) string { return fmt.Sprintf("пайплайн #%d отменён", id) }
	pp.NoLogJobs = "нет упавших или выполняющихся джоб"
	pp.PrevOn = func(ref string) string { return "на " + ref }
	pp.PrevStagesLoading, pp.PrevNoStages = "◐ загружаю стейджи…", "стейджей нет"

	pp.Crumb = "Pipelines"
	pp.Summary = func(jobs, parts string) string { return jobs + ": " + parts }
	pp.PanelStage, pp.PanelRunner, pp.PanelAttempts, pp.PanelAllowFailure = "стейдж", "раннер", "попытки", "allow_failure"
	pp.PanelNoLog = "(лог пуст)"
	pp.BridgeWaiting, pp.BridgeNoAccess = "ждёт", "нет доступа"
	pp.Downstream = "downstream"
	pp.YAMLErrors = "Ошибки конфигурации пайплайна"
	pp.ToastNoAccess = "Нет доступа к downstream-проекту"
	pp.NoJobs = "нет джоб"
	pp.JobRetried = func(n string) string { return "перезапускаю " + n }
	pp.JobCanceled = func(n string) string { return n + " отменена" }
	pp.JobPlayed = func(n string) string { return n + " запущена" }
	pp.CancelJobTitle = func(n string) string { return "Отменить джобу " + n + "?" }
	pp.CancelJobBody = "Джоба будет остановлена."
	pp.PlayTitle = func(n string) string { return "Запуск " + n }
	pp.NotManual = "запускать можно только manual-джобы"
	pp.AttemptsTitle = func(n string) string { return "Попытки " + n }
	pp.AttemptLatest = "последняя"
	pp.AttemptsKeys = "j/k выбор · enter лог · esc закрыть"

	f := &m.Form
	f.ColKey, f.ColValue, f.ColType = "KEY", "VALUE", "тип"
	f.VarsEmpty = "переменных нет — джоба запустится как есть"
	f.VarsHint = "a добавить · d удалить · enter правка · space тип"
	f.RunHint = "ctrl+s запустить · esc отмена"
	f.TypeEnv, f.TypeFile = "env_var", "file"
	f.RunTitle, f.RefLabel, f.VarsLabel = "Запуск пайплайна", "Ref", "Переменные"
	f.RefCurrent, f.RefTag = "текущая в git", "тег"
	f.RefSearching, f.NoRefs = "ищу…", "нет подходящих веток и тегов"
	f.HintsLabel, f.HintsLoading, f.HintsNone = "Из .gitlab-ci.yml:", "загружаю подсказки из .gitlab-ci.yml…", "в .gitlab-ci.yml нет объявленных переменных"
	f.HintsUnavailable = "подсказки недоступны"
	f.WillCreate = func(ref string, n int) string {
		return fmt.Sprintf("будет создан: ref=%s, variables=%d", ref, n)
	}
	f.RefRequired = "Укажи ветку или тег"
	f.PickValue = func(key string) string { return "Значение для " + key }
	f.PickHint = "1-9 добавить подсказку · tab — другой блок"
	f.Created = func(id int64) string { return fmt.Sprintf("пайплайн #%d создан", id) }
	f.RunNoVars = "переменных нет"
	f.RunButton = "Запустить"
	f.RunKeys = "↑/↓ ref · enter принять · tab блок · esc отмена"

	lg := &m.Log
	lg.Follow, lg.NoFollow = "● follow", "○ пауза"
	lg.Updated = func(age string) string { return "обновл. " + age }
	lg.Truncated = "… начало обрезано, o — открыть полный лог в браузере"
	lg.Empty = func(status string) string { return "лог пуст: джоба " + status }
	lg.SearchPrompt = "/"
	lg.MatchOf = func(i, n int) string { return fmt.Sprintf("совпадение %d из %d", i, n) }
	lg.NoMatches = "совпадений нет"
	lg.SearchKeys = "n / N далее / назад"
	lg.Crumb = "лог"
	lg.Attempt = func(i, n int) string { return fmt.Sprintf("попытка %d/%d", i, n) }

	r := &m.MR
	r.ColIID, r.ColTitle, r.ColAuthor, r.ColBranch = "IID", "Заголовок", "Автор", "Ветка"
	r.ColPipeline, r.ColAppr, r.ColThr, r.ColUpd = "Пайплайн", "Апр", "Тр", "Обн"
	r.Draft = "черновик"
	r.Conflicts = "⚠ conflicts"
	r.EmptyBase = func(state string) string {
		switch state {
		case "merged":
			return "Нет смерженных MR"
		case "closed":
			return "Нет закрытых MR"
		case "all":
			return "Нет MR"
		}
		return "Нет открытых MR"
	}
	r.EmptyWhere = ", где "
	r.QualReviewerMe = "ты ревьюер"
	r.QualAuthorMe = "ты автор"
	r.QualAssigneeMe = "ты исполнитель"
	r.QualMine = "ты автор, исполнитель или ревьюер"
	r.QualDraft = "это черновики"
	r.QualNotDraft = "это не черновики"
	r.QualReviewer = func(u string) string { return "ревьюер @" + u }
	r.QualLabels = func(l string) string { return "метки " + l }
	r.QualText = func(t string) string { return fmt.Sprintf("в заголовке есть %q", t) }
	r.EmptyHint = "/ меняет фильтры · m — фильтр «я»"
	r.PrevApprovals = func(names string) string { return "одобрили " + names }
	r.PrevNoApprovals = "апрувов пока нет"
	r.PrevPipeline = "пайплайн"
	r.PrevDiff = func(add, del, files int) string {
		return fmt.Sprintf("+%d −%d в %s", add, del, Count(RU, files, PluralForms{"файле", "файлах", "файлах"}))
	}
	r.PrevNoDesc = "нет описания"

	r.Crumb = "MRs"
	r.TabOverview, r.TabDiscussions, r.TabPipelines = "Обзор", "Обсуждения", "Пайплайны"
	r.Opened = func(age string) string { return "открыт " + age + " назад" }
	r.Merged = func(by, age string) string {
		if by == "" {
			return "смержен " + age + " назад"
		}
		return "смержил " + by + " " + age + " назад"
	}
	r.Closed = func(age string) string { return "закрыт " + age + " назад" }
	r.ChipApprovals = func(have, req int) string {
		if req > 0 && have < req {
			return fmt.Sprintf("%d/%d апрувов", have, req)
		}
		return fmt.Sprintf("апрувов: %d", have)
	}
	r.ChipThreads = func(n int) string {
		return fmt.Sprintf("%s не разрешено", Count(RU, n, PluralForms{"тред", "треда", "тредов"}))
	}
	r.SecApprovals, r.SecLabels, r.SecPipeline, r.SecLastThread = "АПРУВЫ", "МЕТКИ", "ПАЙПЛАЙН", "ПОСЛЕДНИЙ ТРЕД"
	r.None, r.ThreadResolved, r.ThreadUnresolved = "нет", "разрешён", "не разрешён"
	r.EmptyDiscussions, r.EmptyPipelines, r.Loading = "Обсуждений нет", "У MR нет пайплайнов", "◐ загружаю…"
	r.ColStatus, r.ColID, r.ColRef, r.ColSource, r.ColCommit, r.ColUser = "Статус", "ID", "Ref", "Источник", "Коммит", "Автор"
	r.ColDur, r.ColAgo = "Длит", "Назад"
	r.NeedApprovals = func(left int) string { return fmt.Sprintf("нужно ещё %d", left) }

	r.ToastApproved = func(iid int64) string { return fmt.Sprintf("одобрено !%d", iid) }
	r.ToastUnapproved = func(iid int64) string { return fmt.Sprintf("одобрение снято !%d", iid) }
	r.ToastMerged = func(iid int64) string { return fmt.Sprintf("смержено !%d", iid) }
	r.ToastAutoMerge = func(iid int64) string {
		return fmt.Sprintf("!%d смержится, когда пайплайн пройдёт", iid)
	}
	r.MergePushed = "В ветку запушили, обнови (r)"
	r.CannotMerge = func(status string) string { return "Нельзя смержить: " + status }
	r.RebaseTitle = func(iid int64) string { return fmt.Sprintf("Сделать rebase !%d?", iid) }
	r.RebaseBody = "Source-ветка будет перебазирована на target-ветку на сервере."
	r.RebaseDone, r.RebaseStillRunning = "rebase выполнен", "rebase ещё идёт"
	r.RebaseFailed = func(reason string) string { return "rebase не удался: " + reason }
	r.RebaseChip = "◐ rebase…"
	r.MergeTitle = func(iid int64) string { return fmt.Sprintf("Merge !%d", iid) }
	r.MergeNow, r.MergeWhenPipeline = "Merge сейчас", "Merge, когда пайплайн пройдёт"
	r.MergeSquash, r.MergeRemoveBranch = "Squash коммитов", "Удалить source-ветку"
	r.MergeReason = func(status, label string) string {
		if label == "" {
			return "Нельзя смержить: " + status
		}
		return "Нельзя смержить: " + status + " — " + label
	}
	r.MergeHint = "←/→ способ · space переключить · enter merge · esc отмена"
	r.MergeHintBlocked = "esc закрыть"
	r.CheckoutFailed = func(code int) string {
		return fmt.Sprintf("glab завершился с ошибкой (код %d)", code)
	}
	r.GlabNotLoggedIn = "glab не залогинен"
	r.StatusLabel = func(s string) string {
		switch s {
		case "mergeable":
			return "можно мержить"
		case "conflict":
			return "есть конфликты"
		case "need_rebase":
			return "нужен rebase"
		case "ci_still_running":
			return "пайплайн ещё идёт"
		case "ci_must_pass":
			return "пайплайн должен пройти"
		case "not_approved":
			return "нужны апрувы"
		case "discussions_not_resolved":
			return "есть неразрешённые треды"
		case "draft_status":
			return "черновик"
		case "blocked_status":
			return "заблокирован"
		case "checking":
			return "проверяется"
		case "unchecked":
			return "ещё не проверен"
		case "external_status_checks":
			return "ждёт внешних проверок"
		case "requested_changes":
			return "запрошены изменения"
		case "not_open":
			return "не открыт"
		}
		return ""
	}
}
