/* Database created using psql for tests - query_test */
DROP TABLE pages;
CREATE TABLE pages (
    id SERIAL NOT NULL,
    title text,
    text text,
    keywords text,
    status integer,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    url text,
    summary text
);


insert into pages (title,text,keywords,status,created_at,updated_at,url,summary) VALUES('Title 1.','test 1 text','keywords1',100,NOW(),NOW(),'test.example.com','');
insert into pages (title,text,keywords,status,created_at,updated_at,url,summary) VALUES('Title 2','test 2 text','keywords 2',100,NOW(),NOW(),'test.example.com','');
insert into pages (title,text,keywords,status,created_at,updated_at,url,summary) VALUES('Title 3 here','test 3 text','keywords,3',100,NOW(),NOW(),'test.example.com','');
