# Built from the sources, no JDK or Maven needed on the machine:
#   docker build -t ghcr.io/orgkeep/greetings-api-vertx .
# (the demo compose builds it with docker compose up --build)

FROM maven:3.9-eclipse-temurin-25 AS build
WORKDIR /build
# the dependencies first, in their own layer: rebuilt only when the pom changes
COPY pom.xml .
RUN mvn -q -B dependency:go-offline
COPY src src
# no fat jar: the classes, and the dependencies copied next to them
RUN mvn -q -B package dependency:copy-dependencies -DincludeScope=runtime -DskipTests

FROM eclipse-temurin:25-jre
WORKDIR /app
COPY --from=build /build/target/classes classes
COPY --from=build /build/target/dependency lib
EXPOSE 8291
# not root
USER 1000
ENTRYPOINT ["java", "-cp", "classes:lib/*", "io.orgkeep.demo.greetings.GreetingsApi"]
