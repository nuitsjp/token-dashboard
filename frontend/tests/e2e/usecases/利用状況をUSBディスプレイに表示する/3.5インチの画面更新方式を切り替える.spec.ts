import { test, expect } from '@playwright/test';
import { chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { saveLargeDisplayFixture, startServer } from '../../support/server';
import { localHome } from '../../support/local';

const smallID = 'USB\\VID_1A86&PID_5722&E2E\\USB35INCHIPSV2';
const settingsFileName = 'settings.json';
type SavedPresentation = {
  reverse180: boolean;
  blankOnUpdate?: boolean;
  skipFull5h: boolean;
  services: Record<string, Record<string, { enabled: boolean; content: string }>>;
};
type SavedSettings = { source: string; displayID: string; displayName: string; limitStyle: string; presentation: SavedPresentation };

test('3.5インチの更新方式を保存し、再起動後も復元する', async ({ page, context }) => {
  test.setTimeout(180_000);
  const dataDir = mkdtempSync(join(tmpdir(), 'turzx-blank-update-e2e-'));
  const settingsFile = join(dataDir, settingsFileName);
  const { home, env } = localHome();
  const services = {
    Local: {
      Claude: { enabled: true, content: 'Both' },
      Codex: { enabled: false, content: 'Limits' },
    },
  };
  writeFileSync(settingsFile, JSON.stringify({
    source: 'Local', displayID: smallID, displayName: 'TURZX E2E 3.5-inch', limitStyle: 'gauges-35-portrait',
    presentation: { reverse180: true, skipFull5h: false, services },
  }));
  let server = await startServer(dataDir, 34160, env);
  const file = () => JSON.parse(readFileSync(settingsFile, 'utf8')) as SavedSettings;
  const style = () => page.getByRole('textbox', { name: 'Display style', exact: true });
  const output = () => page.getByRole('textbox', { name: 'Output device', exact: true });
  const rotate = () => page.getByRole('switch', { name: 'Rotate 180°', exact: true });
  const blank = () => page.getByRole('switch', { name: 'Blank on update', exact: true });
  const restart = async () => {
    await page.close();
    await server.stop();
    server = await startServer(dataDir, 34160, env);
    page = await context.newPage();
    await page.goto(server.url);
  };
  try {
    await test.step('分岐条件', async () => {
      await page.goto(server.url);
      await expect(output()).toHaveValue(/3\.5-inch.*Disconnected/);
      await expect(style()).toHaveValue('Gauges 3.5" Portrait');
      expect(file().source).toBe('Local');
      expect(file().displayID).toBe(smallID);
      expect(file().limitStyle).toBe('gauges-35-portrait');
      expect(Object.hasOwn(file().presentation, 'blankOnUpdate')).toBe(false);
    });

    await test.step('手順1', async () => {
      await expect(rotate()).toBeVisible();
      await expect(blank()).toBeVisible();
      await expect(rotate()).toHaveAttribute('aria-checked', 'true');
      await expect(blank()).toHaveAttribute('aria-checked', 'false');
      const rotateBox = (await rotate().boundingBox())!;
      const blankBox = (await blank().boundingBox())!;
      expect(blankBox.x).toBeGreaterThan(rotateBox.x + rotateBox.width);
      expect(Math.abs(blankBox.y - rotateBox.y)).toBeLessThan(8);
    });

    await test.step('手順2', async () => {
      await blank().click();
      await expect.poll(() => file().presentation.blankOnUpdate).toBe(true);
      await expect(blank()).toHaveAttribute('aria-checked', 'true');
      expect(file().presentation.reverse180).toBe(true);
      expect(file().presentation.services).toEqual(services);

      const beforeOn = readFileSync(settingsFile);
      chmodSync(settingsFile, 0o444);
      try {
        await blank().press('Space');
        await expect(page.getByRole('alert').first()).toBeVisible();
        await expect(blank()).toHaveAttribute('aria-checked', 'true');
        expect(file().presentation.blankOnUpdate).toBe(true);
        expect(readFileSync(settingsFile).equals(beforeOn)).toBe(true);
      } finally {
        chmodSync(settingsFile, 0o600);
      }

      await blank().press('Space');
      await expect.poll(() => file().presentation.blankOnUpdate).toBe(false);
      await expect(blank()).toHaveAttribute('aria-checked', 'false');
      await expect(page.getByRole('alert')).toHaveCount(0);
      expect(file().presentation.reverse180).toBe(true);
      expect(file().presentation.services).toEqual(services);

      const beforeOff = readFileSync(settingsFile);
      chmodSync(settingsFile, 0o444);
      try {
        await blank().press('Space');
        await expect(page.getByRole('alert').first()).toBeVisible();
        await expect(blank()).toHaveAttribute('aria-checked', 'false');
        expect(file().presentation.blankOnUpdate).toBe(false);
        expect(readFileSync(settingsFile).equals(beforeOff)).toBe(true);
      } finally {
        chmodSync(settingsFile, 0o600);
      }

      await blank().press('Space');
      await expect.poll(() => file().presentation.blankOnUpdate).toBe(true);
      await expect(blank()).toHaveAttribute('aria-checked', 'true');
      await expect(page.getByRole('alert')).toHaveCount(0);
      expect(file().presentation.reverse180).toBe(true);
      expect(file().presentation.services).toEqual(services);
    });

    await test.step('手順3', async () => {
      await restart();
      await expect(style()).toHaveValue('Gauges 3.5" Portrait');
      await expect(blank()).toHaveAttribute('aria-checked', 'true');
      expect(file().presentation.blankOnUpdate).toBe(true);

      await blank().press('Space');
      await expect.poll(() => file().presentation.blankOnUpdate).toBe(false);
      await expect(blank()).toHaveAttribute('aria-checked', 'false');
      await restart();
      await expect(blank()).toHaveAttribute('aria-checked', 'false');
      expect(file().presentation.blankOnUpdate).toBe(false);

      await blank().press('Space');
      await expect.poll(() => file().presentation.blankOnUpdate).toBe(true);
      await expect(blank()).toHaveAttribute('aria-checked', 'true');
      await page.close();
      await server.stop();
      const largeID = saveLargeDisplayFixture(dataDir);
      const large = file();
      large.limitStyle = 'gauges';
      writeFileSync(settingsFile, JSON.stringify(large));
      server = await startServer(dataDir, 34160, env);
      page = await context.newPage();
      await page.goto(server.url);
      expect(largeID).toBe(file().displayID);
    });

    await test.step('受け入れ条件', async () => {
      await expect(style()).toHaveValue('Gauges');
      await expect(output()).toHaveValue(/9\.2-inch.*Disconnected/);
      await expect(blank()).toHaveCount(0);
      const saved = file();
      expect(saved.source).toBe('Local');
      expect(saved.limitStyle).toBe('gauges');
      expect(saved.presentation.blankOnUpdate).toBe(true);
      expect(saved.presentation.reverse180).toBe(true);
      expect(saved.presentation.services).toEqual(services);
    });
  } finally {
    chmodSync(settingsFile, 0o600);
    await page.close().catch(() => undefined);
    await server.stop();
    rmSync(dataDir, { recursive: true, force: true });
    rmSync(home, { recursive: true, force: true });
  }
});
