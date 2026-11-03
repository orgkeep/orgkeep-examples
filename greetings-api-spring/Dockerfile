# Built from the sources, no JDK or Maven needed on the machine:
#   docker build -t ghcr.io/orgkeep/greetings-api-spring .
# (the demo compose builds it with docker compose up --build)

FROM maven:3.9-eclipse-temurin-25 AS build
WORKDIR /build
# the dependencies first, in their own layer: rebuilt only when the pom changes
COPY pom.xml .
RUN mvn -q -B dependency:go-offline
COPY src src
RUN mvn -q -B package -DskipTests

FROM eclipse-temurin:25-jre
WORKDIR /app
COPY --from=build /build/target/greetings-api-spring-*.jar app.jar
EXPOSE 8290
# not root
USER 1000
ENTRYPOINT ["java", "-jar", "app.jar"]
