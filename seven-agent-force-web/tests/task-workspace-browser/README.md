# Task workspace browser fixture

Run from `seven-agent-force-web`: `pnpm exec vite --config tests/task-workspace-browser/vite.config.ts`.
Open `http://127.0.0.1:5178/?scenario=question`. The labeled fixture imports the
production conversation, operation/delivery cards, status rail, milestones and CSS.
Its own Vite config replaces three transport modules with in-memory test adapters.
It never authenticates, connects to the business API, calls a Provider or changes Git.

Choose `question`, `operation`, `delivery`, `review`, `success`, `failed`, `empty`
or `unavailable`. The 1200/390 buttons render the same components in a precisely
sized iframe, so CSS media queries use that frame viewport. Check history scrolling,
visible composer, option selection, draft retention after a failed submit, return,
question closure, refusal/skip confirmation, and results-tab visibility.
These checks are browser component acceptance, not live business acceptance.
