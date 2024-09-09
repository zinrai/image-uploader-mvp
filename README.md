# image-uploader-mvp

image-uploader is a Go-based web application that provides image upload, thumbnail generation, and display functionality.

## Features

- Image upload (supports JPG and PNG)
- Automatic thumbnail generation for uploaded images
- Display of recently uploaded images
- Prevention of duplicate image uploads

## PostgreSQL Setup

4. Create database:
   ```sql
   CREATE DATABASE image_uploader;
   ```

5. Create user and grant privileges:
   ```sql
   CREATE USER image_uploader_user WITH PASSWORD 'your_secure_password';
   GRANT ALL PRIVILEGES ON DATABASE image_uploader TO image_uploader_user;
   ```

6. Connect to the new database:
   ```sql
   \c image_uploader
   ```

7. Create table:
   ```sql
   CREATE TABLE images (
       id SERIAL PRIMARY KEY,
       filename VARCHAR(255) NOT NULL,
       thumbnail_filename VARCHAR(255) NOT NULL,
       width INTEGER NOT NULL,
       height INTEGER NOT NULL,
       sha256sum CHAR(64) NOT NULL UNIQUE,
       upload_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
   );
   ```

8. Grant privileges on the images table to image_uploader_user:
   ```sql
   GRANT ALL PRIVILEGES ON TABLE images TO image_uploader_user;
   GRANT USAGE, SELECT ON SEQUENCE images_id_seq TO image_uploader_user;
   ```

9. Exit PostgreSQL:
   ```
   \q
   ```

## Application Setup

1. Install dependencies:
   ```
   $ go mod tidy
   ```

2. Copy `config.yaml.example` to `config.yaml` and edit database connection info:
   ```
   $ cp config.yaml.example config.yaml
   ```

4. Create necessary directories:
   ```
   $ mkdir image thumb
   ```

5. Run the application:
   ```
   $ go run main.go
   ```

The application should now be running at http://localhost:8080.

## Usage

- To upload an image: Send a POST request to the `/upload` endpoint with the image file as multipart form data.
- To view the image list: Access http://localhost:8080/view in your browser.

## License

This project is licensed under the MIT License - see the [LICENSE](https://opensource.org/license/mit) for details.
