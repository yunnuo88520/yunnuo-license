package com.yunnuo.license;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import com.yunnuo.license.model.ActivateRequest;
import com.yunnuo.license.model.HeartbeatRequest;
import com.yunnuo.license.model.RenewRequest;
import com.yunnuo.license.model.UnbindRequest;
import com.yunnuo.license.model.VerifyRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class LicenseClientTest {
    private final ObjectMapper mapper = LicenseClient.defaultObjectMapper();
    private HttpServer server;

    @AfterEach
    void stopServer() {
        if (server != null) {
            server.stop(0);
        }
    }

    @Test
    void lifecycleRoutesAndApiErrors() throws Exception {
        List<String> paths = new ArrayList<>();
        server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/v1/licenses/", exchange -> {
            JsonNode input = mapper.readTree(exchange.getRequestBody());
            assertEquals("app_test", input.path("app_key").asText());
            assertEquals("product/2.0", exchange.getRequestHeaders().getFirst("User-Agent"));
            String path = exchange.getRequestURI().getPath();
            paths.add(path);
            if ("bad".equals(input.path("card_code").asText())) {
                send(exchange, 404, """
                        {"success":false,"request_id":"req_error","error":{"code":"CARD_INVALID","message":"card invalid"}}
                        """);
                return;
            }
            String data;
            if (path.endsWith("heartbeat")) {
                data = "{\"accepted\":true,\"server_time\":\"2026-08-12T03:00:00Z\"}";
            } else if (path.endsWith("unbind")) {
                data = "{\"unbound\":true,\"license_no\":\"lic_test\",\"server_time\":\"2026-08-12T03:00:00Z\"}";
            } else {
                data = "{\"license_no\":\"lic_test\",\"status\":\"active\",\"server_time\":\"2026-08-12T03:00:00Z\"}";
            }
            send(exchange, 200, "{\"success\":true,\"data\":" + data + "}");
        });
        server.start();

        LicenseClient client = LicenseClient.builder(baseUrl(), "app_test")
                .userAgent("product/2.0")
                .build();
        assertEquals("lic_test", client.activate(new ActivateRequest("YN-TEST", "device", "machine-A", "PC", "1.0")).licenseNo());
        assertEquals("lic_test", client.verify(new VerifyRequest("lic_test", "device", "machine-A", null)).licenseNo());
        assertTrue(client.heartbeat(new HeartbeatRequest("lic_test", "device", "machine-A")).accepted());
        assertEquals("lic_test", client.renew(new RenewRequest("lic_test", "YN-RENEW", "device", "machine-A")).licenseNo());
        assertTrue(client.unbind(new UnbindRequest("lic_test", "device", "machine-A", "test")).unbound());

        APIException error = assertThrows(APIException.class,
                () -> client.activate(new ActivateRequest("bad", "device", "machine-A", null, null)));
        assertTrue(error.hasCode("CARD_INVALID"));
        assertEquals(404, error.httpStatus());
        assertEquals("req_error", error.requestId());
        assertEquals(List.of(
                "/v1/licenses/activate", "/v1/licenses/verify", "/v1/licenses/heartbeat",
                "/v1/licenses/renew", "/v1/licenses/unbind", "/v1/licenses/activate"), paths);
    }

    @Test
    void responseLimitAndTimeout() throws Exception {
        server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/v1/licenses/verify", exchange -> {
            JsonNode input = mapper.readTree(exchange.getRequestBody());
            if ("large".equals(input.path("license_no").asText())) {
                byte[] body = new byte[2 * 1024 * 1024 + 1];
                exchange.sendResponseHeaders(200, body.length);
                exchange.getResponseBody().write(body);
                exchange.close();
                return;
            }
            try {
                Thread.sleep(200);
                send(exchange, 200, "{\"success\":true,\"data\":{}}");
            } catch (InterruptedException error) {
                Thread.currentThread().interrupt();
            }
        });
        server.start();
        LicenseClient client = LicenseClient.builder(baseUrl(), "app_test")
                .timeout(Duration.ofMillis(20))
                .build();
        assertThrows(LicenseSDKException.class,
                () -> client.verify(new VerifyRequest("lic", "device", "machine", null)));
        LicenseClient limitClient = LicenseClient.builder(baseUrl(), "app_test").build();
        LicenseSDKException oversized = assertThrows(LicenseSDKException.class,
                () -> limitClient.verify(new VerifyRequest("large", "device", "machine", null)));
        assertTrue(oversized.getMessage().contains("exceeds 2 MiB"));
    }

    private String baseUrl() {
        return "http://127.0.0.1:" + server.getAddress().getPort();
    }

    private static void send(HttpExchange exchange, int status, String body) throws IOException {
        byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        exchange.sendResponseHeaders(status, bytes.length);
        exchange.getResponseBody().write(bytes);
        exchange.close();
    }
}
