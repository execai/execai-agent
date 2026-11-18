# T05 — Vision (drag, Ctrl+V, multi-image)

**Где работает:** ExecAI с vision-моделью (claude-sonnet, gpt-5-vision),
Z.ai с GLM-V, Anthropic с claude-sonnet. claude-cli — теоретически да
(если defaultModel = sonnet), но history передаётся как plain-text промт
с тегами → images теряются. **Документировано как ограничение.**

## Шаги (ExecAI с claude-sonnet)

1. `/source execai`, `/model claude-sonnet-4-6`
2. Сделать скриншот: `gnome-screenshot -a -f /tmp/s1.png`
3. **Drag:** перетащить /tmp/s1.png в окно терминала → должен попасть
   как attachment, индикатор "1 image" под textarea
4. Послать "что на картинке" → ответ с описанием
5. **Ctrl+V:** скопировать ещё картинку (Print Screen → буфер обмена),
   Ctrl+V в textarea → "2 images" (если не очистили после п.4) или "1"
6. Multi-image: drag 3 PNG → "3 images", послать "сравни эти три" →
   модель видит все три

## Pass-criteria
- Drag, Ctrl+V добавляют картинки как attachments
- Multi-image работает (до лимита провайдера)
- Redis-кэш images[] не теряется при ретрае запроса
