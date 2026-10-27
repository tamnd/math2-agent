---
tag: EX3D7S
kind: exercise
name: The last digit of a large power of 7
topics: [3EYFQZ]
about: [M8XBZ4]
difficulty: 2
math_group: B
check: exact
---

## Exercise

Find the last decimal digit of $7^{222}$.

## Hint

Compute $\varphi(10)$.

## Answer

9

## Solution

1. $\gcd(7, 10) = 1$ and $\varphi(10) = 4$, so $7^4 \equiv 1 \pmod{10}$ by Euler's theorem.
2. $222 = 4 \cdot 55 + 2$, so $7^{222} \equiv 7^2 = 49 \equiv 9 \pmod{10}$.
