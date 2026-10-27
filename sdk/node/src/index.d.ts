export interface ReleaseTarget {os:string;arch:string;runtime:string;channel:string;kind:"archive"|"installer"|"container_archive";min_version?:string;max_version?:string;image_tag?:string;image_digest?:string}
export interface InstanceCredential {session_id?:string;instance_id?:string;instance_secret?:string;device_id?:string;seat_id?:string;bindings?:Record<string,string>;target?:ReleaseTarget;current_version?:string}
export interface SeatLease {id:string;instance_id:string;session_id:string;expires_at:string;borrowed:boolean;token:string}
export interface AuthorizationTerms {version:number;card_purpose?:string;forms:string[];bind_mode:string;binding_kinds?:string[];include_apex:boolean;subdomain_depth:"one"|"all";max_bindings:number;require_instance:boolean;max_deployments:number;max_devices:number;max_concurrent_seats:number;seat_lease_seconds:number;offline_grace_days:number;license_mode:"subscription"|"perpetual";update_duration_days:number;expire_grace_days:number;unbind_limit:number;unbind_cooldown_hours:number;features:string[]}
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

export interface ActivateRequest extends InstanceCredential {
  card_code: string;
  bind_mode: string;
  bind_value: string;
  device_name?: string;
  client_version?: string;
}

export interface VerifyRequest extends InstanceCredential {
  license_no: string;
  bind_mode: string;
  bind_value: string;
  license_token?: string;
}

export interface ProductRelease {
 target?:ReleaseTarget;
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
  target?: ReleaseTarget;
  currentVersion?: string;
}

export function verifyReleaseManifest(release: ProductRelease, expected: ReleaseExpectation): Record<string, unknown>;
export function verifyReleasePackage(release: ProductRelease, content: Uint8Array, expected: ReleaseExpectation): Record<string, unknown>;

export interface HeartbeatRequest extends InstanceCredential {
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
 bind_mode?:string;bind_value?:string;authorization_terms?:AuthorizationTerms;
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
  cardRequirements(cardCode:string,options?:RequestOptions):Promise<{appid:string;name:string;authorization_terms:AuthorizationTerms;duration_days:number;is_permanent:boolean}>;
  acquireSeat(input:VerifyRequest & {session_id:string;borrow_days?:number},options?:RequestOptions):Promise<SeatLease>;
  releaseSeat(input:VerifyRequest,options?:RequestOptions):Promise<{accepted:boolean}>;
  createClaimTicket(input:VerifyRequest,options?:RequestOptions):Promise<{claim_token:string;expires_at:string}>;
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
 instanceId?:string;seatId?:string;sessionId?:string;
  licenseNo?: string;
  bindMode: string;
  bindValue: string;
}

export interface OfflineCacheClaims {
  instance_id?:string;seat_id?:string;session_id?:string;purpose?:string;can_use?:boolean;authorization_terms?:AuthorizationTerms;
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
  verifyLicenseToken(token:string,expected:OfflineExpectation):Record<string,any>;
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
export function enforceWebLicense(client: LicenseClient, binding: VerifyRequest, response: {writeHead(status:number,headers:Record<string,string>):unknown;end(body?:string):unknown}, request?: {headers: Record<string,string|string[]|undefined>;socket?: {remoteAddress?: string}}, options?: {trustedProxies?: string[]}): Promise<boolean>;

export interface HighWaterStore {
  load(): Date | string | null | undefined | Promise<Date | string | null | undefined>;
  save(value: Date): void | Promise<void>;
}

export class HighWaterGuard {
  constructor(options: { store: HighWaterStore; allowedRollbackMs?: number });
  checkAndUpdate(current?: Date | string): Promise<void>;
}

export interface RuntimeOptions extends ClientOptions {
 productId:string;productCode:string;publicKeys:Record<number,string>;stateDir:string;registration?:string|Record<string,string>;
 deviceId?:string|(()=>string|Promise<string>);label?:string;currentVersion?:string;target?:ReleaseTarget;
 cacheSeconds?:number;allowOffline?:boolean;highWaterGuard?:HighWaterGuard;clock?:()=>Date;
}
export interface RuntimeContext {request?:{headers:Record<string,string|string[]|undefined>;socket?:{remoteAddress?:string}};host?:string;ip?:string;trustedProxies?:string[]}
export function requestHostname(request:RuntimeContext["request"],options?:{trustedProxies?:string[]}):string;
export class RuntimeLicense {
 static open(options:RuntimeOptions):Promise<RuntimeLicense>;
 attach(licenseNo:string,bindMode:string,registration:string):Promise<LicenseResponse>;
 securityDecision(context?:RuntimeContext):Promise<SecurityDecision>;
 activate(cardCode:string,registration?:string|Record<string,string>):Promise<LicenseResponse>;
 verify(context?:RuntimeContext):Promise<LicenseResponse & {offline?:boolean}>;
 requireFeature(feature:string,context?:RuntimeContext):Promise<LicenseResponse>;
 heartbeat(context?:RuntimeContext):Promise<HeartbeatResponse>;
 renew(cardCode:string,context?:RuntimeContext):Promise<LicenseResponse>;
 claimTicket(context?:RuntimeContext):Promise<{claim_token:string;expires_at:string}>;
 borrowSeat(days:number,context?:RuntimeContext):Promise<SeatLease>;
 releaseSeat(context?:RuntimeContext):Promise<void>;
 checkUpdate(context?:RuntimeContext):Promise<CheckUpdateResponse>;
 downloadUpdate(release:ProductRelease,context?:RuntimeContext):Promise<Uint8Array>;
 authorizeUpdate(releaseID:string,context?:RuntimeContext):Promise<{authorized:boolean;release_id:string}>;
}

export function bindingForWebRequest(binding: VerifyRequest, request: {headers: Record<string,string|string[]|undefined>;socket?: {remoteAddress?: string}}, options?: {trustedProxies?: string[]}): VerifyRequest;
