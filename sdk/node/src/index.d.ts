export interface ClientOptions {
  baseUrl: string;
  appKey: string;
  fetch?: typeof globalThis.fetch;
  timeoutMs?: number;
  userAgent?: string;
}

export interface RequestOptions {
  signal?: AbortSignal;
}

export interface ActivateRequest {
  card_code: string;
  bind_mode: string;
  bind_value: string;
  device_name?: string;
  client_version?: string;
}

export interface VerifyRequest {
  license_no: string;
  bind_mode: string;
  bind_value: string;
  license_token?: string;
}

export interface HeartbeatRequest {
  license_no: string;
  bind_mode: string;
  bind_value: string;
}

export interface RenewRequest extends HeartbeatRequest {
  renew_card_code: string;
}

export interface UnbindRequest extends HeartbeatRequest {
  reason?: string;
}

export interface LicenseResponse {
  license_no: string;
  status: string;
  expired_at?: string;
  grace_until?: string;
  license_token?: string;
  offline_token?: string;
  server_time: string;
}

export interface HeartbeatResponse {
  accepted: boolean;
  server_time: string;
}

export interface UnbindResponse {
  unbound: boolean;
  license_no: string;
  server_time: string;
}

export class LicenseClient {
  constructor(options: ClientOptions);
  activate(input: ActivateRequest, options?: RequestOptions): Promise<LicenseResponse>;
  verify(input: VerifyRequest, options?: RequestOptions): Promise<LicenseResponse>;
  heartbeat(input: HeartbeatRequest, options?: RequestOptions): Promise<HeartbeatResponse>;
  renew(input: RenewRequest, options?: RequestOptions): Promise<LicenseResponse>;
  unbind(input: UnbindRequest, options?: RequestOptions): Promise<UnbindResponse>;
}

export class APIError extends Error {
  readonly code: string;
  readonly httpStatus: number;
  readonly requestId: string;
}

export function isAPIErrorCode(error: unknown, code: string): error is APIError;

export class VerificationError extends Error {
  readonly code: string;
}

export function isVerificationErrorCode(error: unknown, code: string): error is VerificationError;

export const VerificationCode: Readonly<{
  INVALID_FILE: "INVALID_FILE";
  INVALID_SIGNATURE: "INVALID_SIGNATURE";
  WRONG_PRODUCT: "WRONG_PRODUCT";
  WRONG_LICENSE: "WRONG_LICENSE";
  BINDING_MISMATCH: "BINDING_MISMATCH";
  LICENSE_EXPIRED: "LICENSE_EXPIRED";
  OFFLINE_WINDOW_EXPIRED: "OFFLINE_WINDOW_EXPIRED";
  ISSUED_IN_FUTURE: "ISSUED_IN_FUTURE";
  CLOCK_ROLLBACK: "CLOCK_ROLLBACK";
}>;

export interface OfflineVerifierOptions {
  publicKeyPem: string | Uint8Array;
  productCode: string;
  clock?: () => Date;
  clockSkewMs?: number;
}

export interface OfflineExpectation {
  licenseNo?: string;
  bindMode: string;
  bindValue: string;
}

export interface OfflineCacheClaims {
  type: "offline_cache";
  license_no: string;
  product_code: string;
  bind_mode: string;
  bind_hash?: string;
  bind_digest: string;
  license_expired_at?: string | null;
  offline_until: string;
  issued_at: string;
  key_version: number;
}

export interface OfflineLicenseClaims {
  version: number;
  key_version: number;
  license_no: string;
  product_id: string;
  product_code: string;
  product_name: string;
  app_key: string;
  bind_mode: "device";
  machine_code: string;
  issued_at: string;
  expired_at?: string | null;
  is_permanent: boolean;
}

export class OfflineVerifier {
  constructor(options: OfflineVerifierOptions);
  verifyOfflineToken(token: string, expected: OfflineExpectation): OfflineCacheClaims;
  verifyLicenseFile(content: string | Uint8Array | object, machineCode: string): OfflineLicenseClaims;
}

export function bindingDigest(bindMode: string, bindValue: string): string;

export interface HighWaterStore {
  load(): Date | string | null | undefined | Promise<Date | string | null | undefined>;
  save(value: Date): void | Promise<void>;
}

export class HighWaterGuard {
  constructor(options: { store: HighWaterStore; allowedRollbackMs?: number });
  checkAndUpdate(current?: Date | string): Promise<void>;
}
