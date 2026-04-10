CREATE TABLE IF NOT EXISTS albums (
    album_id    VARCHAR(36)  NOT NULL,
    title       VARCHAR(255) NOT NULL,
    description TEXT         NOT NULL,
    owner       VARCHAR(255) NOT NULL,
    seq_counter INT          NOT NULL DEFAULT 0,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (album_id)
);

CREATE TABLE IF NOT EXISTS photos (
    photo_id   VARCHAR(36)                                       NOT NULL,
    album_id   VARCHAR(36)                                       NOT NULL,
    seq        INT                                               NOT NULL,
    status     ENUM('processing','completed','failed','deleted') NOT NULL DEFAULT 'processing',
    url        VARCHAR(1024)                                     DEFAULT NULL,
    s3_key     VARCHAR(512)                                      DEFAULT NULL,
    created_at DATETIME                                          NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME                                          NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (photo_id),
    INDEX idx_album_photo (album_id, photo_id),
    CONSTRAINT fk_album FOREIGN KEY (album_id) REFERENCES albums(album_id)
);
