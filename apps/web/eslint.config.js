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
    rules: {
      // A bare negative call assertion runs BEFORE any promise-continuation
      // dispatch, so it passes whether or not the guard it covers works.
      // See docs/tasks/task-019-vacuous-negative-assertions/ and issue #22.
      'no-restricted-syntax': [
        'error',
        {
          selector:
            "CallExpression[callee.object.property.name='not']" +
            '[callee.property.name=/^toHaveBeenCalled/]',
          message:
            'Use expectNoCall(spy) from src/test/expectNoCall — a bare ' +
            'not.toHaveBeenCalled() runs before promise-continuation dispatch ' +
            'and can pass vacuously. See issue #22.',
        },
        {
          selector:
            "CallExpression[callee.property.name='toHaveBeenCalledTimes']" +
            '[arguments.0.value=0]',
          message:
            'toHaveBeenCalledTimes(0) is not.toHaveBeenCalled() spelled ' +
            'differently — use expectNoCall(spy). See issue #22.',
        },
      ],
    },
  },
  {
    // The helper necessarily contains the banned expression, and its own test
    // must contain the bare form to demonstrate the contrast it exists to fix.
    // Must come AFTER the block above, which also matches src/test/**.
    files: ['src/test/expectNoCall.ts', 'src/test/expectNoCall.test.ts'],
    rules: { 'no-restricted-syntax': 'off' },
  },
);
