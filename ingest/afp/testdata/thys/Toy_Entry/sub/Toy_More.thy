theory Toy_More
  imports Toy
begin

primrec sum_to :: "nat \<Rightarrow> nat" where
  "sum_to 0 = 0" | "sum_to (Suc n) = Suc n + sum_to n"

end
