package io.orgkeep.demo.greetings;

import com.nimbusds.jwt.JWTParser;
import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.authentication.AuthenticationManager;
import org.springframework.security.authentication.AuthenticationManagerResolver;
import org.springframework.security.authentication.ProviderManager;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.http.SessionCreationPolicy;
import org.springframework.security.oauth2.jwt.JwtValidators;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;
import org.springframework.security.oauth2.server.resource.authentication.JwtAuthenticationProvider;
import org.springframework.security.oauth2.server.resource.authentication.OpaqueTokenAuthenticationProvider;
import org.springframework.security.oauth2.server.resource.introspection.SpringOpaqueTokenIntrospector;
import org.springframework.security.oauth2.server.resource.web.DefaultBearerTokenResolver;
import org.springframework.security.web.SecurityFilterChain;

import java.text.ParseException;

/**
 * Full and lightweight tokens on the same API, chosen token by token: a token marked "lightweight": true (a convention
 * of Orgkeep) is read by introspection, any other one is verified offline as a JWT. What an API needs while the realm
 * owner switches to lightweight tokens, and afterwards: a full token costs no call to Keycloak.
 * <p>
 * The marker is read before the token is verified, which is safe because both ways verify it: removing the marker of
 * a lightweight token breaks its signature (the JWT way refuses it), adding it to a forged token sends it to Keycloak,
 * which answers "active": false.
 */
@Configuration
public class SecurityConfig {
    private static final Logger log = LoggerFactory.getLogger(SecurityConfig.class);
    private static final String MARKER = "lightweight";

    @Bean
    SecurityFilterChain securityFilterChain(HttpSecurity http,
                                            AuthenticationManagerResolver<HttpServletRequest> tokenResolver) throws Exception {
        return http
                // an API: every call needs a token, no session, no CSRF (no cookie to protect)
                .authorizeHttpRequests(requests -> requests.anyRequest().authenticated())
                .sessionManagement(session -> session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .csrf(csrf -> csrf.disable())
                .oauth2ResourceServer(server -> server.authenticationManagerResolver(tokenResolver))
                .build();
    }

    /**
     * Per request, the way to verify its bearer token. An AuthenticationManagerResolver is the hook Spring Security
     * gives for this (also used for several issuers): it picks an AuthenticationManager from the request.
     */
    @Bean
    AuthenticationManagerResolver<HttpServletRequest> tokenResolver(
            @Value("${keycloak.issuer}") String issuer,
            @Value("${keycloak.url}") String realmUrl,
            @Value("${demo.catalog}") String catalog,
            @Value("${demo.catalog-secret}") String catalogSecret) {
        // full tokens: the keys of Keycloak (JWKS) read once, then the signature, issuer and expiry checked offline.
        // Not withIssuerLocation(realmUrl): it requires the issuer to be the URL the API reaches Keycloak with, false in
        // Docker (tokens issued for localhost:8280, Keycloak reached as keycloak:8080)
        NimbusJwtDecoder decoder = NimbusJwtDecoder.withJwkSetUri(realmUrl + "/protocol/openid-connect/certs").build();
        decoder.setJwtValidator(JwtValidators.createDefaultWithIssuer(issuer));
        AuthenticationManager offline = new ProviderManager(new JwtAuthenticationProvider(decoder));
        // lightweight tokens: introspection with the credentials of the catalog, cached until the token expires
        // (Spring Security does not cache it)
        if (catalogSecret.isBlank()) {
            log.warn("No secret for the catalog {} (CATALOG_CLIENT_SECRET): the lightweight tokens will be refused", catalog);
        }
        AuthenticationManager introspection = new ProviderManager(new OpaqueTokenAuthenticationProvider(
                new CachingIntrospector(SpringOpaqueTokenIntrospector
                        .withIntrospectionUri(realmUrl + "/protocol/openid-connect/token/introspect")
                        .clientId(catalog)
                        .clientSecret(catalogSecret)
                        .build())));

        DefaultBearerTokenResolver bearer = new DefaultBearerTokenResolver();
        return request -> isLightweight(bearer.resolve(request)) ? introspection : offline;
    }

    /** The marker, read without verifying the token (see the class comment). Not a JWT: the JWT way refuses it. */
    private static boolean isLightweight(String token) {
        if (token == null) {
            return false;
        }
        try {
            return Boolean.TRUE.equals(JWTParser.parse(token).getJWTClaimsSet().getBooleanClaim(MARKER));
        } catch (ParseException e) {
            return false;
        }
    }
}
