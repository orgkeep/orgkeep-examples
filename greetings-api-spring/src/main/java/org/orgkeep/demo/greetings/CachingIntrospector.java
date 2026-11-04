package org.orgkeep.demo.greetings;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.security.oauth2.core.OAuth2AuthenticatedPrincipal;
import org.springframework.security.oauth2.core.OAuth2TokenIntrospectionClaimAccessor;
import org.springframework.security.oauth2.server.resource.introspection.OpaqueTokenIntrospector;

import java.time.Instant;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * The introspection of each token, kept until the token expires: the cost depends on the number of tokens, not of
 * requests (a user with a token of 5 minutes costs one call to Keycloak, whatever the number of requests).
 * <p>
 * A minimal cache for the demo: in memory, emptied of the expired tokens as it goes. In a real API, a cache library
 * (Caffeine) with a maximum size. Trade-off: a token revoked in Keycloak stays valid here until it expires; a shorter
 * duration if the revocation matters.
 */
public class CachingIntrospector implements OpaqueTokenIntrospector {
    private static final Logger log = LoggerFactory.getLogger(CachingIntrospector.class);

    private final OpaqueTokenIntrospector delegate;
    private final Map<String, Cached> cache = new ConcurrentHashMap<>();

    private record Cached(OAuth2AuthenticatedPrincipal principal, Instant expiresAt) { }

    public CachingIntrospector(OpaqueTokenIntrospector delegate) {
        this.delegate = delegate;
    }

    @Override
    public OAuth2AuthenticatedPrincipal introspect(String token) {
        Instant now = Instant.now();
        Cached cached = cache.get(token);
        if (cached != null && cached.expiresAt().isAfter(now)) {
            log.debug("Introspection from the cache");
            return cached.principal();
        }

        // an inactive token (expired, revoked, not of this catalog) throws: nothing is cached
        OAuth2AuthenticatedPrincipal principal = delegate.introspect(token);
        log.debug("Introspection call to Keycloak");
        if (principal instanceof OAuth2TokenIntrospectionClaimAccessor claims && claims.getExpiresAt() != null) {
            cache.values().removeIf(entry -> !entry.expiresAt().isAfter(now));
            cache.put(token, new Cached(principal, claims.getExpiresAt()));
        }
        return principal;
    }
}
