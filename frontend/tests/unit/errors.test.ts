import { describe, it, expect } from 'vitest';
import { publicError } from '../../src/shared/errors';
describe('Public error contract', () => {
  it('uses structured Go cause without leaking internal stack', () => {
    const result = publicError({ message: 'internal password', cause: { code: 'VALIDATION', message: '入力を確認してください。', fieldErrors: { title: '必須' } } });
    expect(result.code).toBe('VALIDATION'); expect(result.fieldErrors?.title).toBe('必須');
    expect(JSON.stringify(result)).not.toContain('password');
  });
  it('does not trust arbitrary exceptions as public messages', () => {
    expect(publicError(new Error('secret URL')).message).not.toContain('secret');
  });
});
