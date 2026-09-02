from fastapi import FastAPI

from typing import Set, Tuple

from app.levels import levels
from app.models import Level, GetLevelResponse, RunLevelRequest, RunLevelResponse, GetLevelsInfoResponse
from app.award import Award
from app.field import Field
from app.robot import Robot, NO_MOVEMENT
from app.utils import validate
from copy import copy, deepcopy

MAX_LENGTH_OF_SOURCE_CODE = 1000

app = FastAPI()


@app.get("/api/levels", response_model=GetLevelsInfoResponse)
async def get_levels_info():
    return {
        'count': len(levels)
    }

def init_robot_sensors(level: dict):
    robots_coords = set()
    vertical_walls = { (wall['x'], wall['y']) for wall in level['walls'] if wall['type'] == 'vertical' }
    horizontal_walls = { (wall['x'], wall['y']) for wall in level['walls'] if wall['type'] == 'horizontal'}
    for robot in level['robots']:
        robots_coords.add((robot['x'], robot['y']))
    for robot in level['robots']:
        up = (
            (robot['x'], robot['y'] - 1) in robots_coords
            or robot['y'] == 0
            or (robot['x'], robot['y']) in horizontal_walls
        )
        down = (
            (robot['x'], robot['y'] + 1) in robots_coords
            or robot['y'] == level['height'] - 1
            or (robot['x'], robot['y'] + 1) in horizontal_walls
        )
        left = (
            (robot['x'] - 1, robot['y']) in robots_coords
            or robot['x'] == 0 or
            (robot['x'], robot['y']) in vertical_walls
        )
        right = (
            (robot['x'] + 1, robot['y']) in robots_coords
            or robot['x'] == level['width'] - 1
            or (robot['x'] + 1, robot['y']) in vertical_walls
        )
        sensors = {
            'up': up,
            'down': down,
            'left': left,
            'right': right,
        }
        robot.update({'sensors': sensors})

@app.get("/api/level/{level_id}", response_model=GetLevelResponse)
async def get_level(level_id: int):
    if level_id >= len(levels):
        return {
            'state': 'error',
            'reason': 'level not found'
        }
    level = deepcopy(levels[level_id])
    level.update({'id': level_id})
    init_robot_sensors(level)
    return {
        'state': 'success',
        'level': level
    }

@app.post("/api/level/run", response_model=RunLevelResponse)
async def run_level(request: RunLevelRequest):
    if len(request.code) > MAX_LENGTH_OF_SOURCE_CODE:
        # TODO
        pass
    if request.level_id >= len(levels):
        return {
            'state': 'error',
            'reason': 'level not found'
        }

    level_dict = deepcopy(levels[request.level_id])
    level_dict.update({'id': request.level_id})
    init_robot_sensors(level_dict)
    level = Level(**level_dict)

    validate_result = validate(request.code)
    if not validate_result['ok']:
        return {
            'state': 'error',
            'reason': validate_result['reason']
        }


    field=Field(
        width=level.width,
        height=level.height,
        robots=[
            Robot(x=r.x, y=r.y, velocity=NO_MOVEMENT)
            for r in level.robots
        ],
        awards=[
            Award(r.x, r.y) for r in level.awards
        ],
        wincondition_check=level_dict['wincondition_check'],
        vertical_walls={(w.x, w.y) for w in level.walls if w.type == 'vertical'},
        horizontal_walls={(w.x, w.y) for w in level.walls if w.type == 'horizontal'},
        prepared_user_code=request.code
    )
    steps = [
        {
            'number': 0,
            'robots': [
                {'x': robot.x, 'y': robot.y}
                for robot in level.robots]
        }
    ]
    try:
        while field.can_do_steps:
            field.fill_sensors()
            for i, robot in enumerate(field.robots):
                steps[-1]['robots'][i]['sensors'] = copy(robot.sensors)
            field.make_step()

            step_record = {
                'number': field.steps,
                'robots': [{'x': robot.x, 'y': robot.y} for robot in field.robots]
            }
            steps.append(step_record)

        field.fill_sensors()
        for i, robot in enumerate(field.robots):
            steps[-1]['robots'][i]['sensors'] = copy(robot.sensors)

    except Exception as e:
        return {
            'state': 'error',
            'reason': str(e)
        }
    return {
        'state': 'success',
        'result': field.result,
        'steps': steps
    }
