from typing import Dict, Tuple

from dataclasses import dataclass, field
from uuid import uuid4, UUID

@dataclass(frozen=True)
class Velocity:
    x: int
    y: int
    description: str

RIGHT = Velocity(1, 0, '→')
LEFT = Velocity(-1, 0, '←')
UP = Velocity(0, -1, '↑')
DOWN = Velocity(0, 1, '↓')
NO_MOVEMENT = Velocity(0, 0, '')

MOVEMENTS = {
    'UP': UP,
    'DOWN': DOWN,
    'LEFT': LEFT,
    'RIGHT': RIGHT,
    'NO_MOVEMENT': NO_MOVEMENT
}

@dataclass
class Robot:
    x: int = 0
    y: int = 0
    prev_x: int = 0
    prev_y = 0
    velocity: Velocity = NO_MOVEMENT
    movable: bool = True
    sensors: Dict = field(default_factory=dict)

    uuid: UUID = field(default_factory=uuid4, repr=False)

    def make_step(self, step: int):
        if self.movable:
            self.prev_x = self.x
            self.prev_y = self.y
            self.x += self.velocity.x
            self.y += self.velocity.y

    def make_step_back(self):
        self.x = self.prev_x
        self.y = self.prev_y

    def collides(self, other: 'Robot'):
        return self.uuid != other.uuid and self.x == other.x and self.y == other.y

    def __eq__(self, other: Tuple[int, int]):
        return self.x == other[0] and self.y == other[1]
