import { test, expect } from '@playwright/test';

test('入力不正と画面遷移の取り消しで下書きを維持する', async ({ page }) => {
  const body = page.getByLabel('本文', { exact: true });
  await test.step('分岐条件', async () => {
    await page.goto('/#/notes');
    await page.getByRole('link', { name: '新しいメモ' }).click();
    await body.fill('未保存の内容');
  });
  await test.step('手順1', async () => {
    await page.getByRole('button', { name: '保存する', exact: true }).click();
    await expect(page.getByRole('alert')).toBeVisible();
    await expect(body).toHaveValue('未保存の内容');
  });
  await test.step('手順2', async () => {
    await page.getByRole('link', { name: '02　一括取り込み' }).click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await expect(page.getByText('未保存の変更を破棄して画面を移動しますか？', { exact: true })).toBeVisible();
  });
  await test.step('手順3', async () => {
    await page.getByRole('button', { name: '編集に戻る', exact: true }).click();
    await expect(body).toHaveValue('未保存の内容');
  });
  await test.step('受け入れ条件', async () => {
    await expect(page).toHaveURL(/#\/notes/);
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(body).toHaveValue('未保存の内容');
  });
});
