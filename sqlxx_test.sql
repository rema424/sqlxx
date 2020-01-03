-- mysql

create database if not exists sqlxxtest;
create user if not exists sqlxxtester@localhost identified by 'Passw0rd!';
grant all privileges on sqlxxtest.* to sqlxxtester@localhost;
