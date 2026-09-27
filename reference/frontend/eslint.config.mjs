import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import hooks from 'eslint-plugin-react-hooks';
import globals from 'globals';
export default tseslint.config(
  { ignores: ['bindings/**', 'src/routeTree.gen.ts', 'dist/**', 'test-results/**', 'playwright-report/**'] },
  js.configs.recommended, ...tseslint.configs.recommended,
  { files: ['**/*.{ts,tsx}'], languageOptions: { globals: { ...globals.browser, ...globals.node, __MOCK__: 'readonly' } },
    plugins: { 'react-hooks': hooks }, rules: { ...hooks.configs.recommended.rules,
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
    } },
  { files: ['src/usecases/**/*.{ts,tsx}', 'src/shared/**/*.{ts,tsx}'], rules: {
    'no-restricted-imports': ['error', { patterns: [
      { group: ['@bindings/**/service', '@bindings/**/service.ts', '@notes-service'], message: 'Use the feature access functions. Generated model types are allowed.' },
    ] }],
  } },
  { files: ['src/features/**/*.{ts,tsx}'], rules: {
    'no-restricted-imports': ['error', { patterns: [{ group: ['**/usecases/**', '**/routes/**'], message: 'Features must not depend on use cases or routes.' }] }],
  } },
);
