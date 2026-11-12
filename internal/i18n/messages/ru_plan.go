// Package messages/ru_plan — Russian strings for the subscription gate and
// /plan. Merged into the "ru" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var ruPlanMessages = map[string]string{
	// === gate ===
	"plan.gate.noPlan": "Модели ExecAI доступны по подписке: любой план, первые 3 дня за 1 ₽.\n" +
		"Оформить: %s\n" +
		"Свои ключи и локальные модели работают без подписки: " +
		"/connect zai|kimi|openai|anthropic|openrouter|custom <ключ>, /source ollama",
	"plan.gate.notLoggedIn": "Для моделей ExecAI войдите: запустите execai и подтвердите вход в браузере (в чате — /login).\n" +
		"Свои ключи работают без входа: " +
		"/connect zai|kimi|openai|anthropic|openrouter|custom <ключ>, /source ollama",
	"plan.gate.fallback": "Бинарь %s не найден — откатываюсь на модели ExecAI.",
	"plan.gate.unverified": "Не удалось проверить подписку ExecAI: биллинг недоступен, а последняя " +
		"успешная проверка старше 7 дней (или её не было).",

	// === /plan ===
	"plan.show.header":         "План ExecAI",
	"plan.show.state":          "  Состояние    : %s",
	"plan.show.tariff":         "  Тариф        : %s",
	"plan.show.validUntil":     "  Действует до : %s",
	"plan.show.checked":        "  Проверено    : %s",
	"plan.show.checkedOffline": "  Проверено    : %s — биллинг недоступен, показан сохранённый ответ",
	"plan.show.allowed":        "  Разрешено    : %s",
	"plan.show.features":       "  Синк и веб-пульт: %s",
	"plan.show.subscribe":      "Оформить: %s",

	"plan.state.active":     "активен",
	"plan.state.trial":      "активен (пробный период)",
	"plan.state.cancelled":  "активен, без автопродления",
	"plan.state.grace":      "грейс: списание не прошло, доступ до %s",
	"plan.state.none":       "нет активного плана",
	"plan.state.flexible":   "нет активного плана (только баланс, не подписка)",
	"plan.state.expired":    "истёк",
	"plan.state.signedOut":  "вход не выполнен",
	"plan.state.unknown":    "неизвестно: биллинг ещё не ответил",
	"plan.state.unverified": "не подтверждён: биллинг недоступен, последняя проверка старше 7 дней",

	"plan.allowed.all":     "все источники",
	"plan.allowed.ownOnly": "свои ключи и локальные модели; модели ExecAI — по подписке",
	"plan.features.yes":    "доступны",
	"plan.features.no":     "нужен план «Работа» или «Макс»",

	// === execai serve ===
	"plan.serve.refused":   "✗ задача %s отклонена проверкой плана, модель не вызывалась",
	"plan.serve.startWarn": "⚠ Задачи на моделях ExecAI будут отклоняться, пока это не решено:",

	"hint.plan": "план ExecAI: состояние и что разрешено",
}

func init() { i18n.Register("ru", ruPlanMessages) }
