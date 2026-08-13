export class APIError extends Error {
  constructor({ code, message, httpStatus, requestId = "" }) {
    const suffix = requestId ? ` (request_id=${requestId})` : "";
    super(`ynlicense: ${code}: ${message}${suffix}`);
    this.name = "APIError";
    this.code = code;
    this.httpStatus = httpStatus;
    this.requestId = requestId;
  }
}

export function isAPIErrorCode(error, code) {
  return error instanceof APIError && error.code === code;
}

export class VerificationError extends Error {
  constructor(code, message) {
    super(`ynlicense: offline verification ${code}: ${message}`);
    this.name = "VerificationError";
    this.code = code;
  }
}

export function isVerificationErrorCode(error, code) {
  return error instanceof VerificationError && error.code === code;
}
