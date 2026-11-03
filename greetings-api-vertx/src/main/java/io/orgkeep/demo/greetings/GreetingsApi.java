package io.orgkeep.demo.greetings;

import io.vertx.core.Vertx;
import io.vertx.core.json.JsonArray;
import io.vertx.core.json.JsonObject;
import io.vertx.ext.web.Router;
import io.vertx.ext.web.RoutingContext;

import java.util.Optional;

/**
 * The greetings API of the realm owner, in Vert.x: says hello, and what the token gives in the catalog of the API.
 * Full and lightweight tokens are accepted, chosen token by token (see TokenVerifier).
 * <p>
 * Configured by environment variables, with the demo of Orgkeep as defaults:
 * KEYCLOAK_ISSUER (http://localhost:8280/realms/demo), KEYCLOAK_URL (the realm as this API reaches it, the issuer by
 * default; http://keycloak:8080/realms/demo in the demo compose), CATALOG (prod), CATALOG_CLIENT_SECRET (no default:
 * generated in the dashboard, page of the catalog), PORT (8291).
 */
public class GreetingsApi {
    private static final System.Logger log = System.getLogger(GreetingsApi.class.getName());

    public static void main(String[] args) {
        String issuer = env("KEYCLOAK_ISSUER", "http://localhost:8280/realms/demo");
        String realmUrl = env("KEYCLOAK_URL", issuer);
        String catalog = env("CATALOG", "prod");
        String catalogSecret = env("CATALOG_CLIENT_SECRET", "");
        int port = Integer.parseInt(env("PORT", "8291"));
        if (catalogSecret.isBlank()) {
            log.log(System.Logger.Level.WARNING,
                    "No secret for the catalog {0} (CATALOG_CLIENT_SECRET): the lightweight tokens will be refused", catalog);
        }

        Vertx vertx = Vertx.vertx();
        TokenVerifier.create(vertx, issuer, realmUrl, catalog, catalogSecret)
                .compose(verifier -> {
                    Router router = Router.router(vertx);
                    router.get("/greetings").handler(ctx -> greet(ctx, verifier, catalog));
                    return vertx.createHttpServer().requestHandler(router).listen(port);
                })
                // String.valueOf: a number would be formatted with a thousands separator (8,291)
                .onSuccess(server -> log.log(System.Logger.Level.INFO, "Greetings API on port {0}",
                        String.valueOf(server.actualPort())))
                .onFailure(e -> {
                    // e.g. Keycloak not reachable: the discovery fails
                    log.log(System.Logger.Level.ERROR, "Could not start the greetings API", e);
                    vertx.close();
                });
    }

    /**
     * Answers how the token was verified, the size of the Authorization header (what the token costs on every call,
     * against the limits of the proxies and servers, often 8 KB a header), the roles of the catalog and its claims.
     */
    private static void greet(RoutingContext ctx, TokenVerifier verifier, String catalog) {
        String header = ctx.request().getHeader("Authorization");
        if (header == null || !header.startsWith("Bearer ")) {
            ctx.response().setStatusCode(401).putHeader("WWW-Authenticate", "Bearer").end();
            return;
        }
        verifier.verify(header.substring("Bearer ".length()))
                .onSuccess(verified -> {
                    JsonObject claims = verified.claims();
                    ctx.json(new JsonObject()
                            .put("greeting", "Hello " + claims.getString("preferred_username"))
                            .put("verification", verified.verification())
                            .put("authorizationHeaderBytes", header.length())
                            .put("rights", catalogRoles(claims, catalog))
                            .put("quotasAndParams", catalogClaims(claims, catalog)));
                })
                .onFailure(e -> {
                    log.log(System.Logger.Level.INFO, "Token refused: {0}", e.getMessage());
                    ctx.response().setStatusCode(401)
                            .putHeader("WWW-Authenticate", "Bearer error=\"invalid_token\"").end();
                });
    }

    /** resource_access.&lt;catalog&gt;.roles, sorted: products, features (product:feature), catalog roles. */
    private static JsonArray catalogRoles(JsonObject claims, String catalog) {
        JsonArray roles = Optional.ofNullable(claims.getJsonObject("resource_access"))
                .map(access -> access.getJsonObject(catalog))
                .map(access -> access.getJsonArray("roles"))
                .orElse(new JsonArray());
        return new JsonArray(roles.stream().map(String::valueOf).sorted().toList());
    }

    /** The flat claims of the catalog, named after it: "prod.billing", "prod.billing.scan"... sorted by name. */
    private static JsonObject catalogClaims(JsonObject claims, String catalog) {
        JsonObject catalogClaims = new JsonObject();
        claims.fieldNames().stream()
                .filter(name -> name.startsWith(catalog + "."))
                .sorted()
                .forEach(name -> catalogClaims.put(name, claims.getValue(name)));
        return catalogClaims;
    }

    private static String env(String name, String defaultValue) {
        String value = System.getenv(name);
        return value == null || value.isBlank() ? defaultValue : value;
    }
}
