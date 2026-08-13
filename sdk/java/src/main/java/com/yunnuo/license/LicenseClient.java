package com.yunnuo.license;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import com.yunnuo.license.model.ActivateRequest;
import com.yunnuo.license.model.HeartbeatRequest;
import com.yunnuo.license.model.HeartbeatResponse;
import com.yunnuo.license.model.LicenseResponse;
import com.yunnuo.license.model.RenewRequest;
import com.yunnuo.license.model.UnbindRequest;
import com.yunnuo.license.model.UnbindResponse;
import com.yunnuo.license.model.VerifyRequest;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

public final class LicenseClient {
    private static final int MAX_RESPONSE_BYTES = 2 * 1024 * 1024;

    private final URI baseUri;
    private final String appKey;
    private final HttpClient httpClient;
    private final ObjectMapper objectMapper;
    private final Duration timeout;
    private final String userAgent;

    public LicenseClient(String baseUrl, String appKey) {
        this(builder(baseUrl, appKey));
    }

    private LicenseClient(Builder builder) {
        this.baseUri = validateBaseUri(builder.baseUrl);
        this.appKey = requireText(builder.appKey, "app key");
        this.httpClient = Objects.requireNonNull(builder.httpClient, "httpClient");
        this.objectMapper = Objects.requireNonNull(builder.objectMapper, "objectMapper");
        this.timeout = Objects.requireNonNull(builder.timeout, "timeout");
        this.userAgent = requireText(builder.userAgent, "user agent");
        if (timeout.isZero() || timeout.isNegative()) {
            throw new IllegalArgumentException("ynlicense: timeout must be positive");
        }
    }

    public static Builder builder(String baseUrl, String appKey) {
        return new Builder(baseUrl, appKey);
    }

    public LicenseResponse activate(ActivateRequest input) {
        return post("/v1/licenses/activate", input, LicenseResponse.class);
    }

    public LicenseResponse verify(VerifyRequest input) {
        return post("/v1/licenses/verify", input, LicenseResponse.class);
    }

    public HeartbeatResponse heartbeat(HeartbeatRequest input) {
        return post("/v1/licenses/heartbeat", input, HeartbeatResponse.class);
    }

    public LicenseResponse renew(RenewRequest input) {
        return post("/v1/licenses/renew", input, LicenseResponse.class);
    }

    public UnbindResponse unbind(UnbindRequest input) {
        return post("/v1/licenses/unbind", input, UnbindResponse.class);
    }

    private <T> T post(String path, Object input, Class<T> responseType) {
        byte[] requestBody;
        try {
            @SuppressWarnings("unchecked")
            Map<String, Object> fields = objectMapper.convertValue(input, LinkedHashMap.class);
            fields.put("app_key", appKey);
            requestBody = objectMapper.writeValueAsBytes(fields);
        } catch (IllegalArgumentException | JsonProcessingException error) {
            throw new LicenseSDKException("ynlicense: encode request", error);
        }

        HttpRequest request = HttpRequest.newBuilder(baseUri.resolve(path))
                .timeout(timeout)
                .header("Accept", "application/json")
                .header("Content-Type", "application/json")
                .header("User-Agent", userAgent)
                .POST(HttpRequest.BodyPublishers.ofByteArray(requestBody))
                .build();
        try {
            HttpResponse<InputStream> response = httpClient.send(request, HttpResponse.BodyHandlers.ofInputStream());
            byte[] raw;
            try (InputStream body = response.body()) {
                raw = readLimited(body, MAX_RESPONSE_BYTES);
            }
            JsonNode envelope = objectMapper.readTree(raw);
            boolean success = envelope.path("success").asBoolean(false);
            if (response.statusCode() < 200 || response.statusCode() >= 300 || !success) {
                JsonNode error = envelope.path("error");
                String code = textOr(error.path("code"), "HTTP_ERROR");
                String message = textOr(error.path("message"), "request failed");
                String requestId = textOr(envelope.path("request_id"), "");
                throw new APIException(code, message, response.statusCode(), requestId);
            }
            JsonNode data = envelope.get("data");
            if (data == null || data.isNull()) {
                throw new LicenseSDKException("ynlicense: response data is missing");
            }
            return objectMapper.treeToValue(data, responseType);
        } catch (APIException | LicenseSDKException error) {
            throw error;
        } catch (InterruptedException error) {
            Thread.currentThread().interrupt();
            throw new LicenseSDKException("ynlicense: request interrupted", error);
        } catch (IOException error) {
            throw new LicenseSDKException("ynlicense: request failed", error);
        }
    }

    private static byte[] readLimited(InputStream input, int limit) throws IOException {
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        byte[] buffer = new byte[8192];
        int total = 0;
        int read;
        while ((read = input.read(buffer)) != -1) {
            total += read;
            if (total > limit) {
                throw new LicenseSDKException("ynlicense: response exceeds 2 MiB limit");
            }
            output.write(buffer, 0, read);
        }
        return output.toByteArray();
    }

    private static URI validateBaseUri(String value) {
        try {
            URI uri = URI.create(requireText(value, "base URL"));
            if (!("http".equalsIgnoreCase(uri.getScheme()) || "https".equalsIgnoreCase(uri.getScheme())) || uri.getHost() == null) {
                throw new IllegalArgumentException();
            }
            String normalized = uri.toString().replaceAll("/+$", "") + "/";
            return URI.create(normalized);
        } catch (IllegalArgumentException error) {
            throw new IllegalArgumentException("ynlicense: base URL must be an absolute HTTP(S) URL", error);
        }
    }

    private static String requireText(String value, String name) {
        if (value == null || value.isBlank()) {
            throw new IllegalArgumentException("ynlicense: " + name + " is required");
        }
        return value.trim();
    }

    private static String textOr(JsonNode node, String fallback) {
        return node != null && node.isTextual() && !node.textValue().isBlank() ? node.textValue() : fallback;
    }

    public static final class Builder {
        private final String baseUrl;
        private final String appKey;
        private HttpClient httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(10))
                .followRedirects(HttpClient.Redirect.NEVER)
                .build();
        private ObjectMapper objectMapper = defaultObjectMapper();
        private Duration timeout = Duration.ofSeconds(10);
        private String userAgent = "yn-license-java/1.0";

        private Builder(String baseUrl, String appKey) {
            this.baseUrl = baseUrl;
            this.appKey = appKey;
        }

        public Builder httpClient(HttpClient value) {
            this.httpClient = value;
            return this;
        }

        public Builder objectMapper(ObjectMapper value) {
            this.objectMapper = value;
            return this;
        }

        public Builder timeout(Duration value) {
            this.timeout = value;
            return this;
        }

        public Builder userAgent(String value) {
            this.userAgent = value;
            return this;
        }

        public LicenseClient build() {
            return new LicenseClient(this);
        }
    }

    static ObjectMapper defaultObjectMapper() {
        return new ObjectMapper()
                .registerModule(new JavaTimeModule());
    }
}
