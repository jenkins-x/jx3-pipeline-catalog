FROM eclipse-temurin:21-jdk
ENV PORT 8080
EXPOSE 8080
COPY build/libs/*.jar /opt/app.jar
WORKDIR /opt
CMD ["java", "-jar", "app.jar"]
