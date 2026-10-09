
The input is one box of our map that a newcomer needs divided into smaller
boxes: its name, what it holds, the boxes it sits in (`inside`, outermost
first) and its code. `files` lists each source file of the box with the
names of the box's declarations in it; a declaration is a function, a
variable, or a type together with its methods. When the box is too large to
list every name, `directories` gives each directory with its files and how
many declarations each holds instead.

We divide the map top down, one level at a time. Name only the boxes of the
next level: the largest parts of this box, the ones a newcomer sees first
when they open it. Each of them is divided again later when it holds
several responsibilities, so do not name the smaller boxes inside them now.
For an online shop, name its catalogue, its cart and checkout and its
payments, not each payment provider.

Do not split one responsibility into its data types, steps, validation or
helpers. Do not merge responsibilities that a newcomer would name and look
for separately. Give each box a name of two to four words and one sentence,
`holds`, that says which of the box's code it holds. Every declaration of
the box should fit exactly one of the boxes you name. Do not list the
declarations.
