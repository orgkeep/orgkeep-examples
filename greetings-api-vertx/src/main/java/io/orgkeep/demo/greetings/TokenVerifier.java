package io.orgkeep.demo.greetings;

import io.vertx.core.Future;
import io.vertx.core.Vertx;
import io.vertx.core.json.JsonObject;
import io.vertx.ext.auth.User;
import io.vertx.ext.auth.authentication.TokenCredentials;
import io.vertx.ext.auth.oauth2.OAuth2Auth;
import io.vertx.ext.auth.JWTOptions;
import io.vertx.ext.auth.oauth2.OAuth2Options;

import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Verifies a bearer token, full or lightweight, chosen token by token by the marker "lightweight": true (a convention
 * of Orgkeep).
 * <p>
 * Why two providers: an OAuth2Auth of vertx-auth-oauth2 with the keys of Keycloak validates every JWT offline and
 * never introspects it. A lightweight token is a signed JWT too: it would pass, with
 * nothing in it. The introspection needs a provider without keys, which takes every token as opaque.
 * <p>
 * The marker is read before the token is verified, which is safe because both ways verify it: removing the marker of
 * a lightweight token breaks its signature (the offline way refuses it), adding it to a forged token sends it to
 * Keycloak, which answers "active": false.
 */
public class TokenVerifier {
    private static final System.Logger log = System.getLogger(TokenVerifier.class.getName());

    /** The claims of a verified token, and how it was verified. */
    public record Verified(JsonObject claims, String verification) { }

    private final OAuth2Auth offline;
    private final OAuth2Auth introspection;
    private final Map<String, Cached> cache = new ConcurrentHashMap<>();

    private record Cached(JsonObject claims, long expiresAt) { }

    private TokenVerifier(OAuth2Auth offline, OAuth2Auth introspection) {
        this.offline = offline;
        this.introspection = introspection;
    }

    /**
     * @param issuer        the realm of Keycloak
     * @param catalog       the catalog of the API: its client must be in the audience of the token (both ways)
     * @param catalogSecret the secret of the catalog client, to introspect
     */
    public static Future<TokenVerifier> create(Vertx vertx, String issuer, String catalog, String catalogSecret) {
        // full tokens: the keys of Keycloak (JWKS) loaded once, then the JWT validated offline: signature, issuer,
        // expiry, and the audience must contain the clientId, the catalog. Not through the OpenID discovery
        // (OpenIDConnectAuth.discover): it would also set the introspection endpoint, and a JWT refused offline (a token
        // without the catalog in its audience) would then be introspected, without credentials, for nothing
        OAuth2Auth offlineProvider = OAuth2Auth.create(vertx, new OAuth2Options()
                .setSite(issuer)
                .setJwkPath("/protocol/openid-connect/certs")
                .setClientId(catalog)
                .setJWTOptions(new JWTOptions().setIssuer(issuer)));
        Future<OAuth2Auth> offline = offlineProvider.jWKSet().map(loaded -> offlineProvider);
        // lightweight tokens: no key, so every token is taken as opaque and introspected with the credentials of the
        // catalog (HTTP Basic). Keycloak only lets the catalog client introspect the tokens of its catalog
        OAuth2Auth introspection = OAuth2Auth.create(vertx, new OAuth2Options()
                .setSite(issuer)
                .setIntrospectionPath("/protocol/openid-connect/token/introspect")
                .setClientId(catalog)
                .setClientSecret(catalogSecret));
        return offline.map(provider -> new TokenVerifier(provider, introspection));
    }

    public Future<Verified> verify(String token) {
        if (!isLightweight(token)) {
            // the decoded claims of the JWT are in the attributes of the user
            return offline.authenticate(new TokenCredentials(token))
                    .map(user -> new Verified(user.attributes().getJsonObject("accessToken"), "jwt (offline)"));
        }

        // Vert.x does not cache the introspection: without a cache, every request would be a call to Keycloak.
        // Kept until the token expires (in memory, for the demo; with a maximum size in a real API)
        long now = System.currentTimeMillis() / 1000;
        Cached cached = cache.get(token);
        if (cached != null && cached.expiresAt() > now) {
            log.log(System.Logger.Level.INFO, "Introspection from the cache");
            return Future.succeededFuture(new Verified(cached.claims(), "introspection"));
        }
        // an inactive token (expired, revoked, not of this catalog) fails: nothing is cached
        return introspection.authenticate(new TokenCredentials(token)).map(user -> {
            log.log(System.Logger.Level.INFO, "Introspection call to Keycloak");
            // the answer of the introspection is the principal of the user
            JsonObject claims = user.principal();
            cache.values().removeIf(entry -> entry.expiresAt() <= now);
            cache.put(token, new Cached(claims, claims.getLong("exp", now)));
            return new Verified(claims, "introspection");
        });
    }

    /** The marker, read without verifying the token (see the class comment). Not a JWT: false, the offline way refuses it. */
    private static boolean isLightweight(String token) {
        String[] parts = token.split("\\.");
        if (parts.length < 2) {
            return false;
        }
        try {
            JsonObject payload = new JsonObject(new String(Base64.getUrlDecoder().decode(parts[1]), StandardCharsets.UTF_8));
            return Boolean.TRUE.equals(payload.getValue("lightweight"));
        } catch (RuntimeException e) {
            return false;
        }
    }
}
