import { test, expect } from '@playwright/test';

test.describe('Settings E2E Tests', () => {
  // Login before testing
  test.beforeEach(async ({ page }) => {
    await page.goto('/admin');
    
    // Check if login form is present
    const loginVisible = await page.getByPlaceholder('Username').isVisible();
    if (loginVisible) {
      await page.getByPlaceholder('Username').fill('admin');
      await page.getByPlaceholder('Password').fill('pw'); // default dev password or from env
      await page.getByRole('button', { name: /Login/i }).click();
    }
    await expect(page.getByText('Settings')).toBeVisible();
  });

  test('Update data retention setting', async ({ page }) => {
    await page.goto('/admin/settings/general');
    
    // Find retention days input (assuming it has a label or can be found)
    const retentionInput = page.locator('input[type="number"]').first();
    await expect(retentionInput).toBeVisible();

    // Change value
    await retentionInput.fill('45');
    
    // Save
    await page.getByRole('button', { name: /Save changes/i }).click();
    
    // Verify saved notification/status
    await expect(page.getByText('Saved')).toBeVisible();

    // Reload page to verify persistence
    await page.reload();
    await expect(retentionInput).toHaveValue('45');

    // Restore original value
    await retentionInput.fill('90');
    await page.getByRole('button', { name: /Save changes/i }).click();
    await expect(page.getByText('Saved')).toBeVisible();
  });

  test('OAuth settings configuration', async ({ page }) => {
    await page.goto('/admin/settings/general');

    const githubIdInput = page.getByPlaceholder('e.g. Iv1.xxx');
    const githubSecretInput = page.getByPlaceholder('Begins with gho_... (or leave blank to keep unchanged)');

    await githubIdInput.fill('test-github-id');
    await githubSecretInput.fill('test-github-secret');

    await page.getByRole('button', { name: /Save changes/i }).click();
    await expect(page.getByText('Saved')).toBeVisible();

    // Reload and verify ID is persisted
    await page.reload();
    await expect(githubIdInput).toHaveValue('test-github-id');
    // Secret should be empty/placeholder on reload as it is encrypted
    await expect(githubSecretInput).toBeEmpty();
  });
});
