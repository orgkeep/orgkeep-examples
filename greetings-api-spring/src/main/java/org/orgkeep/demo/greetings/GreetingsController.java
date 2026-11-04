package org.orgkeep.demo.greetings;

import jakarta.servlet.http.HttpServletRequest;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpHeaders;
import org.springframework.security.core.Authentication;
import org.springframework.security.oauth2.server.resource.authentication.AbstractOAuth2TokenAuthenticationToken;
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationToken;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;

/**
 * The greetings API: says hello, and what the token gives in the catalog of the API. The same code in both modes:
 * the claims come from the JWT (full token) or from the introspection (lightweight token), read the same way.
 */
@RestController
public class GreetingsController {
    private final String catalog;

    public GreetingsController(@Value("${demo.catalog}") String catalog) {
        this.catalog = catalog;
    }

    /**
     * @param verification             how the token was verified: offline (full JWT) or by introspection
     * @param authorizationHeaderBytes the size of the Authorization header of the request: what the token costs on
     *                                 every call, against the limits of the proxies and servers (often 8 KB a header)
     * @param rights                   the roles of the catalog: products, features (product:feature), catalog roles
     * @param quotasAndParams          the claims of the catalog: the quotas and parameters of each product
     *                                 ("prod.billing") and the quotas of each metered feature ("prod.billing.search")
     */
    record Greeting(String greeting, String verification, int authorizationHeaderBytes, List<String> rights,
                    Map<String, Object> quotasAndParams) { }

    @GetMapping("/greetings")
    Greeting greet(Authentication authentication, HttpServletRequest request) {
        // JwtAuthenticationToken for a JWT, BearerTokenAuthentication after an introspection: both give the claims
        Map<String, Object> claims = ((AbstractOAuth2TokenAuthenticationToken<?>) authentication).getTokenAttributes();
        String verification = authentication instanceof JwtAuthenticationToken ? "jwt (offline)" : "introspection";
        String header = request.getHeader(HttpHeaders.AUTHORIZATION);

        return new Greeting("Hello " + claims.get("preferred_username"), verification,
                header == null ? 0 : header.length(), catalogRoles(claims), catalogClaims(claims));
    }

    /** resource_access.&lt;catalog&gt;.roles: nested JSON objects come as maps, either way. */
    private List<String> catalogRoles(Map<String, Object> claims) {
        if (claims.get("resource_access") instanceof Map<?, ?> resourceAccess
                && resourceAccess.get(catalog) instanceof Map<?, ?> catalogAccess
                && catalogAccess.get("roles") instanceof Collection<?> roles) {
            return roles.stream().map(String::valueOf).sorted().toList();
        }
        return List.of();
    }

    /** The flat claims of the catalog, named after it: "prod.billing", "prod.billing.search"... sorted by name. */
    private Map<String, Object> catalogClaims(Map<String, Object> claims) {
        Map<String, Object> catalogClaims = new TreeMap<>();
        claims.forEach((name, value) -> {
            if (name.startsWith(catalog + ".")) {
                catalogClaims.put(name, value);
            }
        });
        return catalogClaims;
    }
}
