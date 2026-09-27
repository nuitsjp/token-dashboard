import { defineConfig, mergeConfig } from 'vitest/config';
import viteConfig from './vite.config';
export default defineConfig(configEnv => mergeConfig(viteConfig(configEnv), { test: {
  environment: 'jsdom', include: ['tests/unit/**/*.test.{ts,tsx}'], setupFiles: ['tests/setup.ts'],
} }));
