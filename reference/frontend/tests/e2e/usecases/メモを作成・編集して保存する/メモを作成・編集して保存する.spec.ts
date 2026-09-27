import { test, expect } from '@playwright/test';

test('保存した内容をGoから再取得し、同じメモを編集できる', async ({ page }) => {
  const saved = page.getByText('保存しました。', { exact: true });
  await test.step('開始条件', async () => {
    await page.goto('/#/notes');
  });
  await test.step('手順1', async () => {
    await page.getByRole('link', { name: '新しいメモ' }).click();
    await expect(page.getByLabel('タイトル', { exact: true })).toHaveValue('');
  });
  await test.step('手順2', async () => {
    await page.getByLabel('タイトル', { exact: true }).fill('E2E 保存確認');
    await page.getByLabel('本文', { exact: true }).fill('Goが永続化する本文');
    await page.getByRole('button', { name: '保存する', exact: true }).click();
    await expect(page).toHaveURL(/id=/);
    await expect(saved).toBeVisible();
    await expect(page.getByRole('link', { name: /E2E 保存確認/ })).toBeVisible();
  });
  await test.step('受け入れ条件', async () => {
    await page.goBack();
    await expect(saved).toHaveCount(0);
    await page.goForward();
    await expect(saved).toHaveCount(0);
    await page.getByRole('link', { name: '新しいメモ', exact: true }).click();
    await expect(saved).toHaveCount(0);
    await page.getByRole('link', { name: /E2E 保存確認/ }).click();
    await expect(saved).toHaveCount(0);
    await page.reload();
    await expect(page.getByLabel('タイトル', { exact: true })).toHaveValue('E2E 保存確認');
    await expect(page.getByLabel('本文', { exact: true })).toHaveValue('Goが永続化する本文');
    await page.getByLabel('本文', { exact: true }).fill('更新した本文');
    await page.getByRole('button', { name: '保存する', exact: true }).click();
    await expect(saved).toBeVisible();
    await page.reload();
    await expect(page.getByLabel('本文', { exact: true })).toHaveValue('更新した本文');
    await expect(page.getByRole('link', { name: /E2E 保存確認/ })).toHaveCount(1);
  });
});
