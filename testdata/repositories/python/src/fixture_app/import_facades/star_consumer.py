from . import star_facade
import fixture_app.import_facades.star_facade as facade_alias


def render(day):
    # The facade declares to_text after its star import: it is the facade's.
    return star_facade.to_text(day), facade_alias.to_text(day)


def rate(day):
    # Only the star binds get_index in the facade, and the star runs after
    # the facade's own shadowed: neither is known without following it.
    return star_facade.get_index(day), star_facade.shadowed(day)
