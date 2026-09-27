export interface PublicError { code: string; message: string; fieldErrors?: Record<string, string> }
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
export function publicError(error: unknown): PublicError {
  const cause = isRecord(error) && isRecord(error.cause) ? error.cause : error;
  if (isRecord(cause) && typeof cause.code === 'string' && typeof cause.message === 'string') {
    const fields: Record<string, string> = {};
    if (isRecord(cause.fieldErrors)) {
      for (const [name, value] of Object.entries(cause.fieldErrors)) {
        if (typeof value === 'string') fields[name] = value;
      }
    }
    return { code: cause.code, message: cause.message, fieldErrors: fields };
  }
  // Cancellation can originate in the local Wails promise before a Go error arrives.
  if (isRecord(error) && (error.name === 'CancelError' || error.name === 'CancelledError' || error.name === 'AbortError')) {
    return { code: 'CANCELLED', message: '中止を要求しました。保存結果は一覧で確認してください。' };
  }
  return { code: 'UNEXPECTED', message: '処理を完了できませんでした。診断ログを確認してください。' };
}
