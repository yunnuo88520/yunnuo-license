export interface ClientOptions {
  baseUrl: string;
  appKey: string;
  fetch?: typeof globalThis.fetch;
  timeoutMs?: number;
  userAgent?: string;
  reportIntegrityFailures?: boolean;
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

export interface ProductRelease {
  id: string;
  product_id: string;
  version: string;
  notes: string;
  status: string;
  filename: string;
  size: number;
  sha256: string;
  key_version: number;
  manifest_token: string;
  created_at: string;
}

export interface CheckUpdateRequest extends VerifyRequest {
  current_version: string;
}

export interface CheckUpdateResponse {
  enabled: boolean;
  update_available: boolean;
  release?: ProductRelease;
}

export interface ReleaseExpectation {
  publicKeyPem: string;
  productId: string;
}

export function verifyReleaseManifest(release: ProductRelease, expected: ReleaseExpectation): Record<string, unknown>;
export function verifyReleasePackage(release: ProductRelease, content: Uint8Array, expected: ReleaseExpectation): Record<string, unknown>;

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
	license_mode: "subscription" | "perpetual";
	can_use: boolean;
	can_update: boolean;
	usage_expired_at: string | null;
	updates_expired_at: string | null;
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
  productKeys(appKey?: string, options?: RequestOptions): Promise<ProductKeyRing>;
  securityDecision(input: VerifyRequest, options?: RequestOptions): Promise<SecurityDecision>;
  reportIntegrity(input: VerifyRequest & {release_id:string;observed_sha256:string}, options?: RequestOptions): Promise<{id:string;status:string}>;
  activate(input: ActivateRequest, options?: RequestOptions): Promise<LicenseResponse>;
  verify(input: VerifyRequest, options?: RequestOptions): Promise<LicenseResponse>;
  heartbeat(input: HeartbeatRequest, options?: RequestOptions): Promise<HeartbeatResponse>;
  renew(input: RenewRequest, options?: RequestOptions): Promise<LicenseResponse>;
  unbind(input: UnbindRequest, options?: RequestOptions): Promise<UnbindResponse>;
  checkUpdate(input: CheckUpdateRequest, options?: RequestOptions): Promise<CheckUpdateResponse>;
  authorizeUpdate(releaseId: string, input: VerifyRequest, options?: RequestOptions): Promise<{ authorized: boolean; release_id: string }>;
  downloadUpdate(release: ProductRelease, input: VerifyRequest, options: RequestOptions & ReleaseExpectation): Promise<Uint8Array>;
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
  KEY_VERSION_UNKNOWN: "KEY_VERSION_UNKNOWN";
}>;

export interface ProductKeyRing {
  product_id: string;
  product_code: string;
  app_key: string;
  current_version: number;
  keys: Array<{product_id:string; key_version:number; public_key_pem:string; created_at:string}>;
}

export interface OfflineVerifierCommonOptions {
  productCode: string;
  clock?: () => Date;
  clockSkewMs?: number;
}

export type OfflineVerifierOptions = OfflineVerifierCommonOptions & (
  {publicKeyPem: string | Uint8Array; publicKeys?: Record<number, string | Uint8Array>} |
  {publicKeys: Record<number, string | Uint8Array>; publicKeyPem?: string | Uint8Array}
);

export interface OfflineExpectation {
  licenseNo?: string;
  bindMode: string;
  bindValue: string;
}

export interface OfflineCacheClaims {
	license_mode?: "subscription" | "perpetual";
	usage_expired_at?: string | null;
	updates_expired_at?: string | null;
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

export function offlineVerifierFromKeyRing(client: LicenseClient, options: RequestOptions & {appKey:string; productCode?:string; clock?:()=>Date; clockSkewMs?:number}): Promise<OfflineVerifier>;

export function bindingDigest(bindMode: string, bindValue: string): string;

export interface SecurityDecision {
  action: "observe" | "deny_updates" | "deny_use" | "block_page" | "authorization_unavailable";
  case_id: string;
  can_use: boolean;
  can_update: boolean;
  message: string;
  support_url: string;
  server_time: string;
}
export function renderLicenseBlockPage(decision: Partial<SecurityDecision>, unavailable?: boolean): string;
export function enforceWebLicense(client: LicenseClient, binding: VerifyRequest, response: {writeHead(status:number,headers:Record<string,string>):unknown;end(body?:string):unknown}): Promise<boolean>;

export interface HighWaterStore {
  load(): Date | string | null | undefined | Promise<Date | string | null | undefined>;
  save(value: Date): void | Promise<void>;
}

export class HighWaterGuard {
  constructor(options: { store: HighWaterStore; allowedRollbackMs?: number });
  checkAndUpdate(current?: Date | string): Promise<void>;
}
