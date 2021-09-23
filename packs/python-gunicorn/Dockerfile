FROM python:3.8

RUN apt-get -y update && \
    apt-get -y upgrade && \
    apt-get install -y --no-install-recommends \
    locales

# Set the locale
RUN sed -i 's/^# *\(en_US.UTF-8\)/\1/' /etc/locale.gen \
    && locale-gen
ENV LANG en_US.UTF-8
ENV LANGUAGE en_US.UTF-8
ENV LC_ALL en_US.UTF-8

ENV PORT 8080
EXPOSE 8080

RUN mkdir -p /app
RUN groupadd -r appuser && useradd -r -s /bin/false -g appuser appuser
WORKDIR /app
RUN chown -R appuser:appuser /app
USER appuser

COPY ./requirements.txt /app
# call pip install before copying app dependencies
# so unless there is a change in dependencies we would like to use container cache for requirements
RUN pip3 install -r requirements.txt

COPY ./api /app/api
COPY ./oasis /app/oasis
COPY app.py .
# TODO COPY files to container here if needed

CMD gunicorn -b 0.0.0.0:8080 -w 3 app:app -t 60