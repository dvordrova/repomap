from sqlalchemy import ForeignKey, ForeignKey as FK
import sqlalchemy as sa
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column


class StoreBase(DeclarativeBase):
    pass


class ArchiveBase(DeclarativeBase):
    pass


class Trade(StoreBase):
    __tablename__ = "trades"
    id: Mapped[int] = mapped_column(primary_key=True)
    order_id: Mapped[int] = mapped_column(ForeignKey("orders.id"))
    note: Mapped[str | None]
    external_name: Mapped[str] = mapped_column("exchange_name")
    alias_order: Mapped[int] = mapped_column(FK("orders.id"))
    qualified_order: Mapped[int] = mapped_column(sa.ForeignKey("orders.id"))

    @classmethod
    def list_rows(cls):
        return "SELECT id, order_id FROM trades JOIN orders ON orders.id = trades.order_id"


class ArchiveTrade(ArchiveBase):
    __tablename__ = "trades"
    __table_args__ = {"schema": "archive"}
    id: Mapped[int] = mapped_column(primary_key=True)


def dynamic_query(table):
    return f"SELECT * FROM {table} JOIN orders ON orders.id = 1"


def joined_query(table):
    return ("SELECT id FROM " + table + " JOIN orders ON orders.id = 1")


def adjacent_query():
    return ("SELECT id "
            "FROM trades JOIN orders ON orders.id = trades.order_id")


def unrelated_text():
    return '''class Fake(StoreBase):
    __tablename__ = "not_a_table"
    id: Mapped[int] = mapped_column(primary_key=True)
'''
