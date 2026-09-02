from copy import deepcopy
from dataclasses import dataclass, field
from typing import List, Set, Tuple, Callable

from app.award import Award
from app.robot import Robot, MOVEMENTS


@dataclass(repr=False)
class Field:
    robots: List[Robot]
    awards: List[Award]
    wincondition_check: Callable
    width: int = 5
    height: int = 5
    initial_count_of_awards: int = field(init=False)

    steps: int = 0
    max_steps: int = 150
    state: str = 'run'
    horizontal_walls: Set[Tuple[int, int]] = field(default_factory=set)
    vertical_walls: Set[Tuple[int, int]] = field(default_factory=set)
    prepared_user_code: str = field(default='None', repr=False)
    help_code: str = field(default='\nrobot.velocity = velocity', repr=False)

    filled_sensors: Set[Tuple[bool, bool, bool, bool]] = field(default_factory=set)

    count_of_no_movements_cycles: int = 0
    max_count_of_no_movements_cycles: int = 5

    def __post_init__(self):
        self.initial_count_of_awards = len(self.awards)

    @property
    def can_do_steps(self):
        return self.state == 'run' and self.steps < self.max_steps and \
            self.count_of_no_movements_cycles < self.max_count_of_no_movements_cycles and \
            any(robot.movable for robot in self.robots)

    def _check_robot(self, robot: Robot):
        if robot.x < 0 or robot.x >= self.width or \
            robot.y < 0 or robot.y >= self.height:
            robot.make_step_back()
            return
        # if step from prev possiotion was not possible through the wall - make step back
        if (
            ((robot.prev_x, robot.prev_y) in self.horizontal_walls and robot.y < robot.prev_y)
            or ((robot.prev_x, robot.prev_y) in self.vertical_walls and robot.x < robot.prev_x)
            or ((robot.x, robot.y) in self.horizontal_walls and robot.y > robot.prev_y)
            or ((robot.x, robot.y) in self.vertical_walls and robot.x > robot.prev_x)
        ):
            robot.make_step_back()
        for other_robot in self.robots:
            if robot.collides(other_robot):
                robot.make_step_back()
                return
        remained_awards = []
        for award in self.awards:
            if award.x == robot.x and award.y == robot.y:
                robot.movable = False
            else:
                remained_awards.append(award)
        self.awards = remained_awards
        if self.wincondition_check(awards=self.awards, filled_sensors=self.filled_sensors):
            self.state = 'stop'

    def _feel_sensors(self, robot: Robot):
        robot.sensors['up'] = robot.y == 0 or (robot.x, robot.y) in self.horizontal_walls
        robot.sensors['right'] = robot.x == self.width - 1 or (robot.x + 1, robot.y) in self.vertical_walls
        robot.sensors['down'] = robot.y == self.height - 1 or (robot.x, robot.y + 1) in self.horizontal_walls
        robot.sensors['left'] = robot.x == 0 or (robot.x, robot.y) in self.vertical_walls

        for other_robot in self.robots:
            if other_robot == (robot.x - 1, robot.y):
                robot.sensors['left'] = True
            if other_robot == (robot.x + 1, robot.y):
                robot.sensors['right'] = True
            if other_robot == (robot.x, robot.y - 1):
                robot.sensors['up'] = True
            if other_robot == (robot.x, robot.y + 1):
                robot.sensors['down'] = True
        self.filled_sensors.add((robot.sensors['up'], robot.sensors['right'], robot.sensors['down'], robot.sensors['left']))

    def fill_sensors(self):
        for robot in self.robots:
            self._feel_sensors(robot)

    def make_step(self):
        """
        determinism
        order of robots araund a cell will be like this
         1
        2 3
         4
        """
        was_movements = False
        for robot in sorted(self.robots, key=lambda robot: (robot.x, robot.y)):
            exec(
                self.prepared_user_code + self.help_code,
                {},
                {'robot': robot, 'step': self.steps, 'velocity': robot.velocity, **MOVEMENTS}
            )
            robot.make_step(self.steps)
            self._check_robot(robot)
            if robot.x != robot.prev_x or robot.y != robot.prev_y:
                was_movements = True
        if not was_movements:
            self.count_of_no_movements_cycles += 1
        else:
            self.count_of_no_movements_cycles = 0
        self.steps += 1

    @property
    def result(self):
        if self.wincondition_check(awards=self.awards, filled_sensors=self.filled_sensors):
            return 'win'
        else:
            return 'lose'
