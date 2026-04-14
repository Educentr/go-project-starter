FROM python:3.11-slim

# Установка рабочей директории
WORKDIR /docs

# Установка зависимостей
RUN pip install --no-cache-dir \
    mkdocs \
    mkdocs-material \
    mkdocs-mermaid2-plugin \
    pymdown-extensions

# Копирование всех файлов проекта
COPY . /docs

# Открытие порта
EXPOSE 8000

# Запуск MkDocs сервера
CMD ["mkdocs", "serve", "--dev-addr=0.0.0.0:8000"]