export interface ApiResponse<T> {
  code: number;
  message?: string;
  traceId?: string;
  data: T;
}

export type DataScopeType = 'ALL' | 'CUSTOM' | 'DEPT' | 'DEPT_AND_CHILD' | 'SELF' | 'NONE';

export interface UserDataScope {
	userId?: API.Int64;
	deptIds: API.Int64[];
	orgIds: API.Int64[];
  scopeType: DataScopeType;
}

export interface LoginUser {
	id?: API.Int64;
  username?: string;
  nickname?: string;
  avatar?: string;
  userAvatar?: string;
  userRole?: string[];
  userPosition?: string[];
  organizations?: string[];
  departments?: string[];
  permissions?: string[];
  roleCodes?: string[];
  postCodes?: string[];
  orgCodes?: string[];
  deptCodes?: string[];
  isAdmin?: boolean;
	primaryOrgId?: API.Int64;
	authVersion?: API.Int64;
  dataScope?: UserDataScope;
}

export interface LoginResponse {
  user?: LoginUser;
  accessToken?: string;
  tokenType?: string;
  accessTtlSec?: number;
  firstLogin?: boolean;
}

export interface ExternalLoginMethod {
  providerCode: string;
  displayName: string;
  icon?: string | null;
  sortOrder: number;
  loginUrl: string;
}

export interface PlatformLoginMethod {
  methodType: 'PASSWORD' | 'PASSKEY' | 'EXTERNAL_OAUTH';
  providerCode: string;
  displayName: string;
  icon?: string | null;
  sortOrder: number;
  loginUrl?: string | null;
}

export interface PlatformLoginBrand {
  title: string;
  subtitle: string;
  theme?: string | null;
}

export interface PlatformLoginMetadata {
  platformCode?: string;
  platformType?: string;
  displayName?: string;
  supportUrl?: string;
}

export interface PlatformLoginOptions {
  loginContextId: string;
  platformName: string;
  brand: PlatformLoginBrand;
  registration?: PlatformRegistrationOptions;
  metadata?: PlatformLoginMetadata;
  methods: PlatformLoginMethod[];
}

export interface PlatformRegistrationOptions {
  formRegisterEnabled?: boolean;
  requireCaptcha?: boolean;
  requiredFields?: string[];
}

export type RuntimePlatformMode = 'local' | 'hub' | 'node';

export interface RuntimePlatformCapabilities {
  controlPlane: boolean;
  federatedHubLogin: boolean;
  nodeApi: boolean;
}

export interface RuntimePlatformFeatures {
  mode: RuntimePlatformMode;
  capabilities: RuntimePlatformCapabilities;
}

export interface RuntimeDockerFeatures {
  enabled: boolean;
}

export interface RuntimeManagedFeature {
  managedByPlatform: boolean;
}

export type RuntimeFeatureCode =
  | 'platform.control'
  | 'federation.hub'
  | 'federation.node'
  | 'docker.admin';

export interface RuntimeFeatureSet {
  enabled: string[];
}

export interface RuntimeFeatures {
  features: RuntimeFeatureSet;
  /** @deprecated Use features.enabled for capability checks. */
  platform: RuntimePlatformFeatures;
  /** @deprecated Use features.enabled for capability checks. */
  docker: RuntimeDockerFeatures;
  notification: RuntimeManagedFeature;
  runtimeLog: RuntimeManagedFeature;
}

export interface ExternalLoginCallbackResult {
  authenticated?: boolean;
  /** BOUND is an identity-link result and must not be treated as a login. */
  outcome?: 'BOUND' | string;
  purpose?: 'IDENTITY_LINK' | string;
  externalIdentityId?: string | number;
  loginTransactionId?: string;
  providerCode?: string;
  redirectUrl?: string;
  userId?: string | number;
}

export interface LoginRequest {
  userAccount: string;
  userPassword: string;
  deviceId?: string;
}

export interface LoginCaptcha {
  challengeIdentifier?: string;
  stepIdentifier?: string;
  imageBase64?: string;
}

export interface LoginPasswordStateRequest {
  loginTransactionId: string;
  userAccount: string;
  loginContextId?: string;
  refreshCaptcha?: boolean;
}

export interface SetupStatus {
  initialized: boolean;
  ownerRequired: boolean;
  loginEnabled: boolean;
  appVersion?: string;
  appCommit?: string;
  startTime?: string;
  setupToken?: string | null;
}

export interface SetupOwnerRequest {
  username: string;
  nickname: string;
  password: string;
  confirmPassword: string;
}

export interface SetupOwnerResult {
  id: API.Int64;
  username: string;
  nickname: string;
  userAvatar?: string;
  permissions?: string[];
  roleCodes?: string[];
  accessToken?: string;
  tokenType?: string;
  accessTtlSec?: number;
}

export interface LoginPasswordState {
  canPasswordLogin: boolean;
  captchaRequired: boolean;
  totpRequired: boolean;
  locked: boolean;
  lockExpiresAt?: number | string | null;
  unlockMethod?: string | null;
  captcha?: LoginCaptcha | null;
}

export interface LoginPasswordSubmitRequest {
  loginTransactionId: string;
  loginContextId?: string;
  userAccount: string;
  password: string;
  captchaCode?: string;
}

export interface LoginPasswordSubmitResult extends LoginPasswordState {
  authenticated: boolean;
  redirectUrl?: string | null;
}

export interface LoginRegisterStateRequest {
  loginTransactionId: string;
  loginContextId: string;
  userAccount: string;
}

export interface LoginRegisterSubmitRequest {
  loginTransactionId: string;
  loginContextId: string;
  userAccount: string;
  userName: string;
  userEmail: string;
  password: string;
  confirmPassword: string;
  emailCode: string;
}

export interface LoginRegisterEmailCodeRequest {
  loginTransactionId: string;
  loginContextId: string;
  userAccount: string;
  userEmail: string;
  captchaCode: string;
}

export interface LoginRegisterState {
  canRegister: boolean;
  captcha?: LoginCaptcha | null;
}

export interface LoginRegisterSubmitResult {
  registered: boolean;
	userId?: API.Int64;
  userAccount?: string;
  message?: string;
  captcha?: LoginCaptcha | null;
}

export interface LoginRegisterEmailCodeResult {
  sent: boolean;
  emailMasked?: string;
  cooldownSeconds: number;
  expiresInSeconds: number;
  message?: string;
  captcha?: LoginCaptcha | null;
}

export interface LoginPasskeyStartRequest {
  loginTransactionId: string;
  loginContextId?: string;
  userAccount: string;
}

export interface LoginPasskeyStartResult {
  challengeIdentifier: string;
  stepIdentifier: string;
  userInterfaceHints?: Record<string, unknown>;
}

export interface LoginPasskeyVerifyRequest {
  loginTransactionId: string;
  userAccount: string;
  credentialIdentifier: string;
  clientDataJSON: string;
  authenticatorData: string;
  signature: string;
}

export interface LoginPasskeyVerifyResult {
  authenticated: boolean;
  redirectUrl?: string | null;
  locked: boolean;
  lockExpiresAt?: number | string | null;
}

export interface LoginTotpVerifyRequest {
  loginTransactionId: string;
  userAccount: string;
  otpCode: string;
}

export interface LoginTotpVerifyResult {
  authenticated: boolean;
  redirectUrl?: string | null;
  locked: boolean;
  lockExpiresAt?: number | string | null;
}

export interface ChallengeStep {
  stepIdentifier?: string;
  challengeType?: string;
  stepPurpose?: string;
  stepState?: string;
  remainingAttemptCount?: number;
  cooldownSeconds?: number;
  switchable?: boolean;
  userInterfaceHints?: Record<string, unknown>;
}

export interface ChallengeRequiredPayload {
  challengeIdentifier?: string;
  flowNonce?: string;
  steps?: ChallengeStep[];
  challengeState?: string;
  effectiveTimeToLiveSeconds?: number;
  requiredAssuranceLevel?: string;
  resolvedAssuranceLevel?: string;
  recommendedStepIdentifier?: string;
  actualChallengeTypeNames?: string[];
}

export interface ChallengeProofHeaders {
  proofToken: string;
  flowNonce: string;
}

export type ChallengeRetryCode =
  | 'CHALLENGE_CANCELLED'
  | 'CHALLENGE_PRESENTER_UNAVAILABLE'
  | 'CHALLENGE_RETRY_EXHAUSTED';

export interface RefreshResponse {
  accessToken?: string;
  tokenType?: string;
  accessTtlSec?: number;
  refreshToken?: string;
}

export interface SsoRuntimeConfig {
  enabled: boolean;
  frontendPrimaryEnabled: boolean;
  resourceServerEnabled: boolean;
  issuer?: string;
  defaultFirstPartyClientId?: string;
}

export interface OidcTokenResponse {
  accessToken?: string;
  tokenType?: string;
  accessTtlSec?: number;
  refreshToken?: string;
  idToken?: string;
  scope?: string;
}

export interface RuntimeLogLine {
  lineId: string;
  logTime?: string;
  level?: string;
  threadName?: string;
  loggerName?: string;
  traceId?: string;
  message?: string;
  source?: Record<string, unknown>;
  fileName?: string;
  lineNumber?: number;
}

export interface RuntimeLogPageData {
  records: RuntimeLogLine[];
  total: number;
  current: number;
  size: number;
  pages?: number;
}

export interface RuntimeLogPageRequest {
  current?: number;
  size?: number;
  keyword?: string;
  contentKeyword?: string;
  level?: string;
  loggerName?: string;
  threadName?: string;
  traceId?: string;
  startTime?: string;
  endTime?: string;
  useRegex?: boolean;
}

export interface RuntimeLogStreamRequest {
  keyword?: string;
  contentKeyword?: string;
  level?: string;
  loggerName?: string;
  threadName?: string;
  traceId?: string;
  lastN?: number;
  useRegex?: boolean;
}
