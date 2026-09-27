// Specification-approval mock for "Hubの接続設定を登録・変更する". It is enabled only
// by WAILS_FRONTEND_MODE=mock and never enters a production build.
import { CancellablePromise } from '@wailsio/runtime';
import type { SaveRequest, View } from '@bindings/token-monitor-turzx/internal/settings/models';

const displays = [
  { deviceID: 'USB\\VID_1CBE&PID_0092\\633A6E01A48A0706', name: 'TURZX1.0 (633A6E01)', connected: true },
  { deviceID: 'USB\\VID_1CBE&PID_0092\\8F21C4D09B3E5A17', name: 'TURZX1.0 (8F21C4D0)', connected: true },
];
let current: View = { url: '', tokenSet: false, displayID: '', displays };
export function Get() { return CancellablePromise.resolve<View>(current); }
export function Save(request: SaveRequest) {
  current = { url: request.url, tokenSet: current.tokenSet || request.token !== '', displayID: request.displayID, displays };
  return CancellablePromise.resolve<View>(current);
}
