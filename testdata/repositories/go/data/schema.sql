CREATE TABLE public.orders (
    id INTEGER PRIMARY KEY,
    customer_id INTEGER REFERENCES customers(id)
);
-- name: ReadOrders :many
SELECT id FROM public.orders JOIN customers ON customers.id = orders.customer_id;
-- Explicit SQL source keeps bare column/alias expressions.
SELECT trading mode;
