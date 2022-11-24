/* Database created using sqlite for tests - query_test */
DROP TABLE pages;
CREATE TABLE pages (
    id integer NOT NULL PRIMARY KEY,
    title text,
    text text,
    keywords text,
    status integer,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    url text,
    summary text
);


insert into pages VALUES(1,'Title 1.','test 1 text','keywords1',100,'2013-03-18 12:18:50.447 +0000','2013-03-18 12:18:50.447 +0000','test.example.com','');
insert into pages VALUES(2,'Title 2','test 2 text','keywords 2',100,'2013-03-18 12:18:50.447 +0000','2013-03-18 12:18:50.447 +0000','test.example.com','');
insert into pages VALUES(3,'Title 3 here','test 3 text','keywords,3',100,'2013-03-18 12:18:50.447 +0000','2013-03-18 12:18:50.447 +0000','test.example.com','');