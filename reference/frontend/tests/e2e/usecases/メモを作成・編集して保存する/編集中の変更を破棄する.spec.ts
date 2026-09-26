import { test, expect } from '@playwright/test';

test('未保存の変更を破棄し、保存済みの内容に戻す', async ({ page }) => {
  const saved = page.getByText('保存しました。', { exact: true });
  await test.step('分岐条件', async () => {
    await page.goto('/#/notes');
    await page.getByRole('link', { name: '新しいメモ' }).click();
    await page.getByLabel('タイトル', { exact: true }).fill('E2E 破棄確認');
    await page.getByLabel('本文', { exact: true }).fill('保存済みの本文');
    await page.getByRole('button', { name: '保存する', exact: true }).click();
    await expect(saved).toBeVisible();
    await page.getByLabel('本文', { exact: true }).fill('一時的な変更');
    await expect(saved).toHaveCount(0);
  });
  await test.step('手順1', async () => {
    await page.getByRole('button', { name: '変更を破棄', exact: true }).click();
    await expect(page.getByLabel('本文', { exact: true })).toHaveValue('保存済みの本文');
    await expect(saved).toHaveCount(0);
  });
  await test.step('受け入れ条件', async () => {
    await page.reload();
    await expect(page.getByLabel('本文', { exact: true })).toHaveValue('保存済みの本文');
  });
});
