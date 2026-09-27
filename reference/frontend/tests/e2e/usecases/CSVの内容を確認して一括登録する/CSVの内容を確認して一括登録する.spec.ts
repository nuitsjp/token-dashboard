import { test, expect } from '@playwright/test';

test('画面間で入力を保持し、確認後に全件を一括登録する', async ({ page }) => {
  const csv = 'title,body\nE2E 取込A,本文A\nE2E 取込B,本文B\n';
  await test.step('開始条件', async () => {
    await page.goto('/#/import');
  });
  await test.step('手順1', async () => {
    await page.getByLabel('CSVデータ', { exact: true }).fill(csv);
    await expect(page.getByText(/現在の保存件数: \d+件/)).toBeVisible();
  });
  await test.step('手順2', async () => {
    await page.getByRole('button', { name: '内容を確認' }).click();
    await expect(page.getByRole('cell', { name: /E2E 取込A/ })).toBeVisible();
  });
  await test.step('手順3', async () => {
    await page.getByRole('link', { name: '入力に戻る' }).click();
    await expect(page.getByLabel('CSVデータ', { exact: true })).toHaveValue(csv);
    await page.getByRole('button', { name: '内容を確認' }).click();
  });
  await test.step('手順4', async () => {
    await page.getByRole('button', { name: '取り込む', exact: true }).click();
    await expect(page.getByText('2件を取り込みました。', { exact: true })).toBeVisible();
  });
  await test.step('受け入れ条件', async () => {
    await page.getByRole('link', { name: '01　メモの編集', exact: true }).click();
    await expect(page.getByRole('link', { name: /E2E 取込A/ })).toBeVisible();
    await page.reload();
    await expect(page.getByRole('link', { name: /E2E 取込B/ })).toBeVisible();
  });
});
