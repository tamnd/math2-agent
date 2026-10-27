-- A small scope store. The category tree is Mathematics > Algebra >
-- Group_theory > Finite_groups > Sporadic_groups, and Mathematics >
-- Mathematics_education, which the categories.yaml of the tests excludes.
CREATE TABLE page (page_id INTEGER, page_namespace INTEGER, page_title TEXT, page_is_redirect INTEGER, page_len INTEGER, page_latest INTEGER);
INSERT INTO page VALUES
 (1,0,'Group_(mathematics)',0,100,11),(2,0,'Lagrange''s_theorem_(group_theory)',0,100,12),
 (3,0,'Groups',1,10,13),(4,0,'Mathematics_(disambiguation)',0,10,14),
 (5,0,'Sylow_theorems',0,100,15),(6,0,'Monster_group',0,100,16),(7,0,'Teaching_methods',0,100,17),
 (8,0,'List_of_group_theory_topics',0,100,18),(9,0,'Coset',0,100,19),(10,0,'Abelian_group',0,100,20),
 (11,0,'Évariste_Galois',0,100,21),
 (100,14,'Mathematics',0,1,0),(101,14,'Algebra',0,1,0),(102,14,'Group_theory',0,1,0),
 (103,14,'Finite_groups',0,1,0),(104,14,'Mathematics_education',0,1,0),(105,14,'Sporadic_groups',0,1,0);
CREATE TABLE redirect (rd_from INTEGER, rd_namespace INTEGER, rd_title TEXT, rd_fragment TEXT);
INSERT INTO redirect VALUES (3,0,'Group_(mathematics)','');
CREATE TABLE props (pp_page INTEGER, pp_propname TEXT, pp_value TEXT);
INSERT INTO props VALUES (4,'disambiguation',''),(1,'wikibase_item','Q83478');
CREATE TABLE cat_targets (lt_id INTEGER, lt_namespace INTEGER, lt_title TEXT);
INSERT INTO cat_targets VALUES (500,14,'Mathematics'),(501,14,'Algebra'),(502,14,'Group_theory'),
 (503,14,'Finite_groups'),(504,14,'Mathematics_education'),(505,14,'Sporadic_groups'),(506,14,'French_mathematicians');
CREATE TABLE subcats (cl_from INTEGER, cl_target_id INTEGER);
INSERT INTO subcats VALUES (101,500),(104,500),(102,501),(103,502),(105,503),(102,500);
