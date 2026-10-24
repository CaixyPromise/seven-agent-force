import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

// The application is not compiled with React Compiler.  eslint-plugin-react-hooks
// v7 exposes compiler-gating diagnostics through its flat "recommended" preset;
// keep the runtime Hooks rules that this checkout actually enforces instead of
// treating an unconfigured compiler as a production lint contract.
const standardReactHooksRules = {
  'react-hooks/rules-of-hooks': 'error',
  'react-hooks/exhaustive-deps': 'warn',
}

export default defineConfig([
  // Nested Git worktrees are independent applications with their own lint baselines.
  // `eslint .` must only validate this checkout's sources.
  globalIgnores(['dist', '.worktrees/**', 'worktree/**']),
  {
    files: ['**/*.{ts,tsx}'],
    plugins: {
      'react-hooks': reactHooks,
    },
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactRefresh.configs.vite,
    ],
    rules: standardReactHooksRules,
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
  },
])
