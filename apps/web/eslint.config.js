import js from '@eslint/js';
import globals from 'globals';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

// ESLint 9 flat config for @myfleet/web. Lints `src` (see the `lint` script).
// typescript-eslint provides TS-aware rules; react-hooks enforces the rules of
// hooks (with exhaustive-deps as an error); react-refresh keeps Fast Refresh
// boundaries clean.
export default tseslint.config(
  { ignores: ['dist'] },
  {
    files: ['**/*.{ts,tsx}'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-hooks/exhaustive-deps': 'error',
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },
  {
    // shadcn/ui primitives and React context modules intentionally co-locate
    // non-component exports (cva variants, the `useFormField`/`useAuth` hooks,
    // context objects) with their components — the canonical shadcn layout.
    // Fast Refresh's component-only rule doesn't apply to these.
    files: ['src/components/ui/**/*.{ts,tsx}', 'src/context/**/*.tsx'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  {
    // Test files run under Vitest globals (describe/it/expect, etc.).
    files: ['**/*.{test,spec}.{ts,tsx}', 'src/test/**/*.{ts,tsx}'],
    languageOptions: {
      globals: { ...globals.node },
    },
  },
);
