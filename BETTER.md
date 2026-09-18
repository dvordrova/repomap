# Could be better

Things that work and are not worth fixing now. Each entry says what is seen,
why it happens and the cheapest way out.

## Map

### Two arrows from one box take the longer way round

Seen in gin-realworld: "Application startup and routing" sends two
initialization arrows from its right side. The one into "Authentication
middleware" leaves by the upper port, climbs over the other and comes down
into the box, crossing it there. Swapping the two ports gives the same single
crossing and two bends fewer: the arrow into the middleware runs straight.

ELK orders the ports of a side by crossings only. With the count equal the
tie is broken by nothing meaningful, and bends come later as a consequence.

Ways out, cheapest first:

- Give ELK the edges of a box sorted by how near their targets are and let
  model order break ties (`elk.layered.considerModelOrder.strategy`). One
  option plus a sort; it moves every layout, so all gallery reports need a
  look.
- After layout, swap the ports of a pair of arrows from one box when the swap
  adds no crossing and removes bends. Narrow, but our code on top of ELK.

Related: initialization arrows take part in the layout although they are
hidden most of the time, so the visible arrows are laid out around wiring the
reader does not see. Laying out the working arrows first and routing the
dashed ones over the finished drawing would make both calmer.
