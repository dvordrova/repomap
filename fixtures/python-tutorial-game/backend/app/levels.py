levels = [
    {
        "width": 7,
        "height": 3,
        "robots": [{"x": 1, "y": 1}],
        "awards": [{"x": 5, "y": 1}],
        "walls": [],
        "wincondition_check": lambda *, awards, **_: len(awards) == 0,
    },
    {
        "width": 10,
        "height": 6,
        "robots": [{"x": 0, "y": 5}, {"x": 9, "y": 5}],
        "awards": [{"x": 4, "y": 3}, {"x": 5, "y": 3}],
        "walls": [],
        "wincondition_check": lambda *, awards, **_: len(awards) == 0,
    },
    {
        "width": 6,
        "height": 11,
        "robots": [{"x": 0, "y": 9}, {"x": 0, "y": 7}, {"x": 0, "y": 5}, {"x": 0, "y": 3}],
        "awards": [{"x": 5, "y": 0}, {"x": 5, "y": 1}, {"x": 5, "y": 2}],
        "walls": [],
        "wincondition_check": lambda *, awards, **_: len(awards) == 0,
    },
    {
        "width": 7,
        "height": 11,
        "robots": [{"x": 3, "y": 9}, {"x": 3, "y": 8}, {"x": 3, "y": 7}, {"x": 3, "y": 6}],
        "awards": [{"x": 5, "y": 0}, {"x": 5, "y": 1}, {"x": 5, "y": 2}],
        "walls": [],
        "wincondition_check": lambda *, awards, **_: len(awards) == 0,
    },
    {
        "width": 10,
        "height": 6,
        "robots": [{"x": 0, "y": 5}, {"x": 9, "y": 4}],
        "awards": [{"x": 4, "y": 3}],
        "walls": [],
        "wincondition_check": lambda *, awards, **_: len(awards) == 0,
    },
    {
        "width": 50,
        "height": 30,
        "robots": [{"x": 10, "y": 13}] * 86 +\
                  [{"x": 20, "y": 15}] * 85,
        "awards": [],
        "walls": [],
        "wincondition_check": lambda *, awards, **_: len(awards) == 0,
    },
    {
        "width": 40,
        "height": 40,
        "robots": [
            {"x": 10, "y": 8},
            {"x": 30, "y": 10},
            {"x": 8, "y": 28},
            {"x": 28, "y": 30}
        ]*100,
        "awards": [],
        "walls": [],
        "wincondition_check": lambda *, awards, **_: len(awards) == 0,
    },
    {
        "width": 4,
        "height": 5,
        "robots": [
            {"x": 1, "y": 1},
        ],
        "awards": [],
        "walls": [
            {"x": 2, "y": 1, "type": "vertical"},
            {"x": 3, "y": 1, "type": "vertical"},
            {"x": 2, "y": 2, "type": "vertical"},
            {"x": 1, "y": 3, "type": "vertical"},

            {"x": 1, "y": 1, "type": "horizontal"},
            {"x": 0, "y": 2, "type": "horizontal"},
            {"x": 1, "y": 2, "type": "horizontal"},
            {"x": 3, "y": 2, "type": "horizontal"},
            {"x": 0, "y": 3, "type": "horizontal"},
            {"x": 1, "y": 4, "type": "horizontal"},
        ],
        "wincondition_check": lambda *, filled_sensors, **_: len(filled_sensors) == 15,
    },
    {
        "width": 3,
        "height": 4,
        "robots": [
            {"x": 0, "y": 0},
            {"x": 1, "y": 1},
        ],
        "awards": [],
        "walls": [
            {"x": 0, "y": 1, "type": "horizontal"},
        ],
        "wincondition_check": lambda *, filled_sensors, **_: len(filled_sensors) == 16,
    }

]
